/*
Package flexpack provides Gradle script parsing utilities for FlexPack build info collection.
gradle_script_parser.go implements a parser for Gradle build scripts that extracts
configuration blocks and URLs from both Groovy DSL (.gradle) and Kotlin DSL (.gradle.kts) files.
*/
package flexpack

import (
	"path/filepath"
	"regexp"
	"strings"

	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack/gradle"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var (
	// \s*\(\s* Matches an opening parenthesis surrounded by optional whitespace
	// \s*=\s*: Matches an equals sign = surrounded by optional whitespace
	// ( ... ) → Capturing Group to save the match text group [1], in this case the script path
	// example: apply(from = "script.gradle")
	// Capture group 1: script path
	applyFromKtsRe = regexp.MustCompile(`(?m)apply\s*\(\s*from\s*=\s*['"]([^'"]+)['"]`)

	// applyFromGroovyRe matches 'apply from: "..."' in Groovy DSL
	// example: apply from: "script.gradle"
	// Capture group 1: script path
	applyFromGroovyRe = regexp.MustCompile(`(?m)apply\s+from\s*:\s*['"]([^'"]+)['"]`)

	// (?:\.set)?  (?:pattern) Non-Capturing Group to apply logic like (| OR operator) without saving into group,  ? Group Optional suffix as url.set (Strict Kotlin DSL),
	// \(\s* An opening parenthesis (,  | OR , \s*=\s* An equals sign surrounded by optional whitespace
	// (?:uri\s*\(\s*)? Optional uri() wrapper
	// example: url = uri("http://...") or url("http://...")
	// Capture group 1: URL string
	urlKtsRe = regexp.MustCompile(`(?m)url(?:\.set)?\s*(?:\(\s*|\s*=\s*)(?:uri\s*\(\s*)?['"]([^'"]+)['"]`)

	// urlGroovyRe matches repository URLs in Groovy DSL
	// example: url "http://..." or url = uri("http://...")
	// Capture group 1: URL string
	urlGroovyRe = regexp.MustCompile(`(?m)url\s*(?:[:=]?\s*|[:=]\s*uri\s*\(\s*)['"]([^'"]+)['"]`)
)

type blockExtractorState struct {
	inString       bool
	stringChar     byte
	inLineComment  bool
	inBlockComment bool
}

func (s *blockExtractorState) processChar(content string, i int) (int, bool) {
	char := content[i]

	if s.inLineComment {
		if char == '\n' {
			s.inLineComment = false
		}
		return i, true
	}

	if s.inBlockComment {
		if char == '*' && i+1 < len(content) && content[i+1] == '/' {
			s.inBlockComment = false
			return i + 1, true
		}
		return i, true
	}

	if s.inString {
		if char == s.stringChar {
			if !buildinfoflexpack.IsEscaped(content, i) {
				s.inString = false
			}
		}
		return i, true
	}

	// Check for start of comments or strings
	switch char {
	case '/':
		if i+1 < len(content) {
			if content[i+1] == '/' {
				s.inLineComment = true
				return i + 1, true
			}
			if content[i+1] == '*' {
				s.inBlockComment = true
				return i + 1, true
			}
		}
	case '"', '\'':
		s.inString = true
		s.stringChar = char
		return i, true
	}

	return i, false
}

func extractAllGradleBlocks(content, keyword string) []string {
	var blocks []string
	idx := 0
	for {
		block, nextIdx := extractNextGradleBlock(content, keyword, idx)
		if nextIdx == -1 {
			break
		}
		if block != "" {
			blocks = append(blocks, block)
		}
		idx = nextIdx
	}
	return blocks
}

func extractNextGradleBlock(content, keyword string, startIndex int) (string, int) {
	state := &blockExtractorState{}
	keywordLen := len(keyword)
	mode := 0
	braceStartIdx := -1
	depth := 0

	// 0: Search for keyword, 1: Search for opening brace, 2: Search for closing brace
	for i := startIndex; i < len(content); i++ {
		newIndex, processed := state.processChar(content, i)
		if processed {
			i = newIndex
			continue
		}

		char := content[i]
		switch mode {
		case 0:
			if char == keyword[0] {
				if i+keywordLen <= len(content) && content[i:i+keywordLen] == keyword {
					validStart := (i == 0) || buildinfoflexpack.IsDelimiter(content[i-1])
					validEnd := (i+keywordLen == len(content)) || buildinfoflexpack.IsDelimiter(content[i+keywordLen])

					if validStart && validEnd {
						mode = 1
						i += keywordLen - 1
					}
				}
			}
		case 1:
			switch char {
			case '{':
				mode = 2
				depth = 1
				braceStartIdx = i
			default:
				if !buildinfoflexpack.IsWhitespace(char) {
					mode = 0
				}
			}
		case 2:
			switch char {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return content[braceStartIdx+1 : i], i + 1
				}
			}
		}
	}
	return "", -1
}

// It attempts to locate the publishing { repositories { ... } } block and find the urls in it
// Covers: url = "..."  ,  url "..."  ,  url = uri("...")  ,  url.set(uri("..."))  ,  maven { url = ... }
func findUrlsInGradleScript(content []byte, isKts bool) [][][]byte {
	contentStr := string(content)
	var combinedRepos string

	collectRepos := func(parentKeyword string) {
		blocks := extractAllGradleBlocks(contentStr, parentKeyword)
		for _, block := range blocks {
			repoBlocks := extractAllGradleBlocks(block, blockRepositories)
			for _, repoBlock := range repoBlocks {
				combinedRepos += repoBlock + "\n"
			}
		}
	}

	collectRepos(blockPublishing)
	// 2. Extract from uploadArchives blocks (legacy maven)
	collectRepos(blockUploadArchives)
	// 3. Extract from dependencyResolutionManagement blocks (Gradle 7.0+)
	collectRepos(blockDepResManagement)

	if combinedRepos == "" {
		return nil
	}
	var re *regexp.Regexp
	if isKts {
		re = urlKtsRe
	} else {
		re = urlGroovyRe
	}
	return re.FindAllSubmatch([]byte(combinedRepos), -1)
}

func collectAppliedScripts(content []byte, isKts bool, props map[string]string, currentScriptPath string) []string {
	var paths []string
	contentStr := string(content)

	var matches [][]string
	if isKts {
		matches = applyFromKtsRe.FindAllStringSubmatch(contentStr, -1)
	} else {
		matches = applyFromGroovyRe.FindAllStringSubmatch(contentStr, -1)
	}

	// Sanitize the current script path to get a clean base directory
	sanitizedScriptDir := ""
	if currentScriptPath != "" {
		sanitizedScriptPath, err := buildinfoflexpack.SanitizePath(currentScriptPath)
		if err == nil {
			sanitizedScriptDir = filepath.Dir(sanitizedScriptPath)
		}
	}

	for _, match := range matches {
		if len(match) > 1 {
			path := match[1]
			path = resolveGradleProperty(path, props)

			if strings.Contains(path, "://") {
				log.Debug("Skipping remote script: " + path)
				continue
			}

			if !filepath.IsAbs(path) && sanitizedScriptDir != "" {
				path = filepath.Join(sanitizedScriptDir, path)
			}

			// Sanitize the resolved path - this returns a new untainted value
			sanitizedPath, err := buildinfoflexpack.SanitizePath(path)
			if err != nil {
				log.Debug("Skipping invalid script path: " + path)
				continue
			}

			paths = append(paths, sanitizedPath)
		}
	}
	return paths
}
