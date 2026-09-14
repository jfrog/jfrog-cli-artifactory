package flexpack

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack/gradle"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var (
	// (?m) Multiline mode, \s* Matches zero or more whitespace, ([^#=\s:]+) Capture Group 1, The characters excluded are # (comments), = (separator), \s (whitespace), and : (separator)
	// (.*) Capture Group 2 matches any character (except newline)
	// example: "key=value" or "key: value"
	// Capture group 1: key
	// Capture group 2: value
	propertiesFileRe = regexp.MustCompile(`(?m)^\s*([^#=\s:]+)\s*[:=]\s*(.*)$`)

	// [a-zA-Z_] name must start with a letter or an underscore, [a-zA-Z0-9_.]* The rest of the name can contain letters, numbers, underscores, or dots
	// \s*=\s* Matches an equals sign = , ['"] Matches an opening quote,  ([^'"]+)  Matches one or more characters that are NOT single or double quotes,  ['"] Matches a closing quote
	// example: myProp = "value"
	// Capture group 1: property name
	// Capture group 2: property value (inside quotes)
	extBlockRe = regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*=\s*['"]([^'"]+)['"]`)

	// (?:project\.)? Optional prefix, It matches project but (?:...) means it is a non-capturing group,
	// ext\. Matches the literal string ext.
	// example: ext.myProp = "value" or project.ext.myProp = "value"
	// Capture group 1: property name
	// Capture group 2: property value (inside quotes)
	extAssignmentRe = regexp.MustCompile(`(?m)^\s*(?:project\.)?ext\.([a-zA-Z_][a-zA-Z0-9_.]*)\s*=\s*['"]([^'"]+)['"]`)

	// \$ Matches the literal dollar sign $ as a prefix
	// [^}] means "Match any character that is not a }, + matches one or more of the preceding element
	// propPlaceHolderRe matches property placeholders like ${propName}
	// example: ${version}
	// Capture group 1: property name ("version")
	propPlaceHolderRe = regexp.MustCompile(`\$\{([^}]+)\}`)

	// propVarRe matches simple property references like $propName
	// Matches: $version or $project.version
	// Capture group 1: property name (e.g., "version" or "project.version")
	propVarRe = regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)`)
)

func collectAllGradleProperties(workingDir string) map[string]string {
	props := make(map[string]string)
	merge := func(source map[string]string) {
		for k, v := range source {
			if v != "" {
				props[k] = v
			}
		}
	}

	// 1. GRADLE_USER_HOME gradle.properties (lowest priority)
	if home := buildinfoflexpack.GetGradleUserHome(); home != "" {
		// GetGradleUserHome already returns a sanitized path
		// Sanitize and validate the properties file path
		propsPath := filepath.Join(home, gradlePropertiesFileName)
		sanitizedPropsPath, err := buildinfoflexpack.SanitizeAndValidatePath(propsPath, home)
		if err == nil {
			merge(readPropertiesFile(sanitizedPropsPath))
		}
	}

	// 2. Project gradle.properties
	sanitizedWorkingDir, err := buildinfoflexpack.SanitizePath(workingDir)
	if err == nil {
		propsFile := filepath.Join(sanitizedWorkingDir, gradlePropertiesFileName)
		// Sanitize and validate the properties file path
		sanitizedPropsFile, err := buildinfoflexpack.SanitizeAndValidatePath(propsFile, sanitizedWorkingDir)
		if err == nil {
			if _, err := os.Stat(sanitizedPropsFile); err == nil {
				merge(readPropertiesFile(sanitizedPropsFile))
			}
		}
	}

	// 3. get project properties from environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, envProjectPrefix) {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				// Extract key after the prefix
				key := strings.TrimSpace(parts[0][gradleEnvPrefixLen:])
				val := strings.TrimSpace(parts[1])
				if key != "" && val != "" {
					props[key] = val
				}
			}
		}
	}

	// 4. CLI Arguments and Options (-P, -D) (highest priority for command line)
	merge(parsePropertiesFromArgs(os.Args))

	// Parse GRADLE_OPTS and JAVA_OPTS
	if opts := os.Getenv(envGradleOpts); opts != "" {
		merge(parsePropertiesFromOpts(opts))
	}
	if opts := os.Getenv(envJavaOpts); opts != "" {
		merge(parsePropertiesFromOpts(opts))
	}
	return props
}

func readPropertiesFile(path string) map[string]string {
	m := make(map[string]string)
	// Sanitize the path before reading
	cleanPath, err := buildinfoflexpack.SanitizePath(path)
	if err != nil {
		return m
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return m
	}

	matches := propertiesFileRe.FindAllSubmatch(content, -1)
	for _, match := range matches {
		if len(match) == 3 {
			key := strings.TrimSpace(string(match[1]))
			val := strings.TrimSpace(string(match[2]))
			val = removeQuotes(val)
			if key != "" && val != "" {
				m[key] = val
			}
		}
	}
	return m
}

func removeQuotes(val string) string {
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		return val[1 : len(val)-1]
	}
	return val
}

func parsePropertiesFromArgs(args []string) map[string]string {
	m := make(map[string]string)

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if (strings.HasPrefix(arg, "-P") || strings.HasPrefix(arg, "-D")) && len(arg) > 2 {
			pair := arg[2:]
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				val = removeQuotes(val)
				if key != "" && val != "" {
					m[key] = val
				}
			}
		}
	}
	return m
}

func parsePropertiesFromOpts(opts string) map[string]string {
	args := splitArgsRespectingQuotes(opts)
	return parsePropertiesFromArgs(args)
}

// splitArgsRespectingQuotes splits a string on whitespace but preserves quoted substrings.
func splitArgsRespectingQuotes(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	var quoteChar byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			current.WriteByte(c)
			if c == quoteChar {
				inQuote = false
			}
		} else {
			switch c {
			case ' ', '\t':
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			case '"', '\'':
				inQuote = true
				quoteChar = c
				current.WriteByte(c)
			default:
				current.WriteByte(c)
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func extractPropertiesFromScript(contentStr string) map[string]string {
	props := make(map[string]string)
	// 1. Extract ext { ... } blocks to define the property value
	extBlocks := extractAllGradleBlocks(contentStr, blockExt)
	for _, block := range extBlocks {
		matches := extBlockRe.FindAllStringSubmatch(block, -1)
		for _, match := range matches {
			if len(match) > 2 {
				props[strings.TrimSpace(match[1])] = match[2]
			}
		}
	}

	// 2. Extract ext.key = "value" or project.ext.key = "value" to define the property key
	matches := extAssignmentRe.FindAllStringSubmatch(contentStr, -1)
	for _, match := range matches {
		if len(match) > 2 {
			props[strings.TrimSpace(match[1])] = match[2]
		}
	}
	return props
}

func resolveGradleProperty(val string, props map[string]string) string {
	if val == "" {
		return val
	}
	// Prevent infinite recursion with a max depth (in case of circular references)
	const maxDepth = 10
	var resolve func(s string, depth int) string
	resolve = func(s string, depth int) string {
		if depth > maxDepth {
			log.Debug("Max recursion depth reached in property resolution for: " + s)
			return s
		}

		// 1. Replace ${key} with value from props
		result := propPlaceHolderRe.ReplaceAllStringFunc(s, func(match string) string {
			// strip ${ and }
			key := match[2 : len(match)-1]
			key = strings.TrimSpace(key)

			if key == "" {
				return match
			}

			switch {
			case strings.HasPrefix(key, "project."):
				// Remove "project." prefix
				key = key[8:]
			case strings.HasPrefix(key, "rootProject."):
				// Remove "rootProject." prefix
				key = key[12:]
			}

			// Also handle property accessors like findProperty("key")
			switch {
			case strings.HasPrefix(key, `findProperty("`) && strings.HasSuffix(key, `")`):
				// Remove findProperty(" and ")
				key = key[14 : len(key)-2]
			case strings.HasPrefix(key, `findProperty('`) && strings.HasSuffix(key, `')`):
				// Remove findProperty(' and ')
				key = key[14 : len(key)-2]
			}

			// Handle escaped ${} - if key starts with $, it might be escaped
			if strings.HasPrefix(key, "$") {
				return match
			}

			if v, ok := props[key]; ok && v != "" {
				if v == match {
					log.Debug("Circular property reference detected for: " + key)
					return match
				}
				return resolve(v, depth+1)
			}
			return match
		})

		// 2. Replace $key with value from props (simple variable syntax)
		// Matches $var where var can contain dots (e.g. $project.version or $host.com)
		result = propVarRe.ReplaceAllStringFunc(result, func(match string) string {
			// Remove $
			fullKey := match[1:]

			// 1. Try exact match
			if v, ok := props[fullKey]; ok && v != "" {
				if v == match {
					return match
				}
				return resolve(v, depth+1)
			}

			// This handles cases like "$host.com" where 'host' is the property
			parts := strings.Split(fullKey, ".")
			for i := len(parts) - 1; i >= 1; i-- {
				prefix := strings.Join(parts[:i], ".")
				// Check if prefix is a known property
				if v, ok := props[prefix]; ok && v != "" {
					if v == "$"+prefix {
						continue
					}
					suffix := "." + strings.Join(parts[i:], ".")
					return resolve(v, depth+1) + suffix
				}
			}
			return match
		})
		return result
	}
	return resolve(val, 0)
}
