package flexpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveGradleProperty(t *testing.T) {
	props := map[string]string{
		"version":      "1.0.0",
		"group":        "com.example",
		"repo":         "libs-release",
		"nested.prop":  "nested-value",
		"project.prop": "ignored-prefix",
		"escaped":      "${escaped}",
	}

	tests := []struct {
		name     string
		val      string
		expected string
	}{
		{"Simple substitution", "${version}", "1.0.0"},
		{"Partial substitution", "prefix-${version}-suffix", "prefix-1.0.0-suffix"},
		{"Multiple substitution", "${group}:${version}", "com.example:1.0.0"},
		{"With project prefix", "${project.version}", "1.0.0"},
		{"With rootProject prefix", "${rootProject.version}", "1.0.0"},
		{"Simple variable", "$version", "1.0.0"},
		{"Dotted variable", "$nested.prop", "nested-value"},
		{"Unknown property", "${unknown}", "${unknown}"},
		{"Circular reference", "${escaped}", "${escaped}"},
		{"findProperty double quotes", `${findProperty("version")}`, "1.0.0"},
		{"findProperty single quotes", `${findProperty('version')}`, "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveGradleProperty(tt.val, props))
		})
	}
}

func TestResolveGradlePropertyExtended(t *testing.T) {
	props := map[string]string{
		"host":          "localhost",
		"port":          "8081",
		"artifactory":   "http://localhost:8081/artifactory",
		"repoKey":       "libs-release",
		"nested.value":  "nestedResult",
		"with.dots.key": "dottedValue",
	}

	tests := []struct {
		name     string
		val      string
		expected string
	}{
		{
			name:     "Empty string",
			val:      "",
			expected: "",
		},
		{
			name:     "No substitution needed",
			val:      "plain-value",
			expected: "plain-value",
		},
		{
			name:     "Build URL from parts",
			val:      "http://${host}:${port}/artifactory/${repoKey}",
			expected: "http://localhost:8081/artifactory/libs-release",
		},
		{
			name:     "Complex nested dots",
			val:      "$with.dots.key",
			expected: "dottedValue",
		},
		{
			name:     "Mixed syntax",
			val:      "${host}:$port",
			expected: "localhost:8081",
		},
		{
			name:     "Partial match with suffix",
			val:      "$host.example.com",
			expected: "localhost.example.com",
		},
		{
			name:     "Multiple same property",
			val:      "${host}/${host}",
			expected: "localhost/localhost",
		},
		{
			name:     "Nested property resolution",
			val:      "${artifactory}/${repoKey}",
			expected: "http://localhost:8081/artifactory/libs-release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveGradleProperty(tt.val, props))
		})
	}
}

func TestResolveGradlePropertyEdgeCases(t *testing.T) {
	t.Run("Empty props map", func(t *testing.T) {
		result := resolveGradleProperty("${missing}", map[string]string{})
		assert.Equal(t, "${missing}", result)
	})

	t.Run("Nil-like empty value in props", func(t *testing.T) {
		props := map[string]string{"key": ""}
		result := resolveGradleProperty("${key}", props)
		assert.Equal(t, "${key}", result) // Empty value should not replace
	})

	t.Run("Special characters in value", func(t *testing.T) {
		props := map[string]string{"special": "value!@#$%^&*()"}
		result := resolveGradleProperty("${special}", props)
		assert.Equal(t, "value!@#$%^&*()", result)
	})

	t.Run("URL as property value", func(t *testing.T) {
		props := map[string]string{"url": "https://example.com:8080/path?query=value"}
		result := resolveGradleProperty("${url}", props)
		assert.Equal(t, "https://example.com:8080/path?query=value", result)
	})

	t.Run("Deeply nested property references", func(t *testing.T) {
		props := map[string]string{
			"a": "${b}",
			"b": "${c}",
			"c": "final",
		}
		result := resolveGradleProperty("${a}", props)
		assert.Equal(t, "final", result)
	})
}

func TestResolveGradlePropertyMoreEdgeCases(t *testing.T) {
	t.Run("Dollar sign not followed by valid identifier", func(t *testing.T) {
		props := map[string]string{"version": "1.0.0"}
		result := resolveGradleProperty("$123invalid", props)
		// $123 is not a valid identifier (starts with number), should not be replaced
		assert.Equal(t, "$123invalid", result)
	})

	t.Run("Empty placeholder", func(t *testing.T) {
		props := map[string]string{"version": "1.0.0"}
		result := resolveGradleProperty("${}", props)
		assert.Equal(t, "${}", result)
	})

	t.Run("Placeholder with only spaces", func(t *testing.T) {
		props := map[string]string{"version": "1.0.0"}
		result := resolveGradleProperty("${   }", props)
		assert.Equal(t, "${   }", result)
	})

	t.Run("Max recursion depth protection", func(t *testing.T) {
		// Create a chain that would exceed max depth
		props := map[string]string{
			"a": "${b}",
			"b": "${c}",
			"c": "${d}",
			"d": "${e}",
			"e": "${f}",
			"f": "${g}",
			"g": "${h}",
			"h": "${i}",
			"i": "${j}",
			"j": "${k}",
			"k": "${l}",
			"l": "final",
		}
		result := resolveGradleProperty("${a}", props)
		// Should stop at max depth and not panic
		assert.NotEmpty(t, result)
	})

	t.Run("Property reference that looks like URL", func(t *testing.T) {
		props := map[string]string{
			"baseUrl": "http://localhost:8081",
		}
		result := resolveGradleProperty("${baseUrl}/artifactory/repo", props)
		assert.Equal(t, "http://localhost:8081/artifactory/repo", result)
	})
}

func TestParsePropertiesFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{"No properties", []string{"clean", "build"}, map[string]string{}},
		{"-P flag joined", []string{"-Pprop=val"}, map[string]string{"prop": "val"}},
		{"-D flag joined", []string{"-Dprop=val"}, map[string]string{"prop": "val"}},
		{"Quotes", []string{"-Pprop=\"val\""}, map[string]string{"prop": "val"}},
		{"Multiple properties", []string{"-Pprop1=val1", "-Dprop2=val2"}, map[string]string{"prop1": "val1", "prop2": "val2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parsePropertiesFromArgs(tt.args))
		})
	}
}

func TestParsePropertiesFromArgsExtended(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name:     "Single quotes",
			args:     []string{"-Pprop='single'"},
			expected: map[string]string{"prop": "single"},
		},
		{
			name:     "URL value",
			args:     []string{"-PartifactoryUrl=http://localhost:8081"},
			expected: map[string]string{"artifactoryUrl": "http://localhost:8081"},
		},
		{
			name:     "Empty value ignored",
			args:     []string{"-Pprop="},
			expected: map[string]string{},
		},
		{
			name:     "Flag followed by another flag",
			args:     []string{"-P", "-D", "something"},
			expected: map[string]string{},
		},
		{
			name:     "Mixed with gradle tasks",
			args:     []string{"clean", "-Prepo=myrepo", "publish", "-Dversion=1.0"},
			expected: map[string]string{"repo": "myrepo", "version": "1.0"},
		},
		{
			name:     "Value with equals sign",
			args:     []string{"-Pconnection=jdbc:mysql://host:3306?param=value"},
			expected: map[string]string{"connection": "jdbc:mysql://host:3306?param=value"},
		},
		{
			name:     "Whitespace in args",
			args:     []string{" -Pprop=val "},
			expected: map[string]string{"prop": "val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parsePropertiesFromArgs(tt.args))
		})
	}
}

func TestReadPropertiesFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name: "Standard properties with equals",
			content: `
key1=value1
key2=value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with colon separator",
			content: `
key1:value1
key2:value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with spaces around equals",
			content: `
key1 = value1
key2  =  value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with comments",
			content: `
# This is a comment
key1=value1
# Another comment
key2=value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with quoted values",
			content: `
key1="quoted value"
key2='single quoted'
`,
			expected: map[string]string{"key1": "quoted value", "key2": "single quoted"},
		},
		{
			name: "URL values",
			content: `
artifactoryUrl=http://localhost:8081/artifactory
repoKey=libs-release-local
`,
			expected: map[string]string{
				"artifactoryUrl": "http://localhost:8081/artifactory",
				"repoKey":        "libs-release-local",
			},
		},
		{
			name:     "Empty file",
			content:  "",
			expected: map[string]string{},
		},
		{
			name: "Values with special characters",
			content: `
password=p@ss=word!123
path=/usr/local/bin
`,
			expected: map[string]string{
				"password": "p@ss=word!123",
				"path":     "/usr/local/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			propsPath := filepath.Join(tmpDir, "test.properties")
			err := os.WriteFile(propsPath, []byte(tt.content), 0644)
			assert.NoError(t, err)

			result := readPropertiesFile(propsPath)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("Non-existent file", func(t *testing.T) {
		result := readPropertiesFile(filepath.Join(tmpDir, "nonexistent.properties"))
		assert.Equal(t, map[string]string{}, result)
	})
}

func TestExtractPropertiesFromScript(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name: "ext block properties",
			content: `
				ext {
					artifactoryUrl = "http://localhost:8081"
					repoKey = "libs-release"
				}
			`,
			expected: map[string]string{
				"artifactoryUrl": "http://localhost:8081",
				"repoKey":        "libs-release",
			},
		},
		{
			name: "ext.key syntax",
			content: `
				ext.version = "1.0.0"
				ext.group = "com.example"
			`,
			expected: map[string]string{
				"version": "1.0.0",
				"group":   "com.example",
			},
		},
		{
			name: "project.ext.key syntax",
			content: `
				project.ext.customProp = "customValue"
			`,
			expected: map[string]string{
				"customProp": "customValue",
			},
		},
		{
			name: "Mixed syntax",
			content: `
				ext {
					prop1 = "value1"
				}
				ext.prop2 = "value2"
				project.ext.prop3 = "value3"
			`,
			expected: map[string]string{
				"prop1": "value1",
				"prop2": "value2",
				"prop3": "value3",
			},
		},
		{
			name:     "No ext properties",
			content:  `version = "1.0.0"`,
			expected: map[string]string{},
		},
		{
			name: "Single quotes",
			content: `
				ext {
					prop = 'singleQuoteValue'
				}
			`,
			expected: map[string]string{
				"prop": "singleQuoteValue",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPropertiesFromScript(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Double quoted value", `"quoted"`, "quoted"},
		{"Single quoted value", `'single'`, "single"},
		{"Empty double quotes", `""`, ""},
		{"Empty single quotes", `''`, ""},
		{"No quotes", "unquoted", "unquoted"},
		{"Single character", "a", "a"},
		{"Mismatched quotes - double then single", `"mismatched'`, `"mismatched'`},
		{"Mismatched quotes - single then double", `'mismatched"`, `'mismatched"`},
		{"Only opening double quote", `"value`, `"value`},
		{"Only closing double quote", `value"`, `value"`},
		{"Nested quotes preserved", `"outer'inner'outer"`, "outer'inner'outer"},
		{"URL value", `"http://example.com"`, "http://example.com"},
		{"Empty string", "", ""},
		{"Just double quote", `"`, `"`},
		{"Just single quote", `'`, `'`},
		{"Whitespace inside quotes", `" value "`, " value "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, removeQuotes(tt.input))
		})
	}
}

func TestSplitArgsRespectingQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple args",
			input:    "-Dprop=value -Pother=val",
			expected: []string{"-Dprop=value", "-Pother=val"},
		},
		{
			name:     "Double quoted value with space",
			input:    `-Dprop="quoted value" -Pother=val`,
			expected: []string{`-Dprop="quoted value"`, "-Pother=val"},
		},
		{
			name:     "Single quoted value with space",
			input:    `-Dprop='quoted value' -Pother=val`,
			expected: []string{`-Dprop='quoted value'`, "-Pother=val"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "Only whitespace",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "Multiple spaces between args",
			input:    "-Dprop=value    -Pother=val",
			expected: []string{"-Dprop=value", "-Pother=val"},
		},
		{
			name:     "Tab separated",
			input:    "-Dprop=value\t-Pother=val",
			expected: []string{"-Dprop=value", "-Pother=val"},
		},
		{
			name:     "Mixed quotes",
			input:    `-Da="value a" -Db='value b'`,
			expected: []string{`-Da="value a"`, `-Db='value b'`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitArgsRespectingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitArgsRespectingQuotesEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Unclosed double quote",
			input:    `-Dprop="unclosed value`,
			expected: []string{`-Dprop="unclosed value`},
		},
		{
			name:     "Unclosed single quote",
			input:    `-Dprop='unclosed value`,
			expected: []string{`-Dprop='unclosed value`},
		},
		{
			name:     "Quote at end",
			input:    `-Dprop=value"`,
			expected: []string{`-Dprop=value"`},
		},
		{
			name:     "Empty quotes",
			input:    `-Dprop=""`,
			expected: []string{`-Dprop=""`},
		},
		{
			name:     "Nested different quotes",
			input:    `-Dprop="value with 'nested' quotes"`,
			expected: []string{`-Dprop="value with 'nested' quotes"`},
		},
		{
			name:     "Multiple quoted args",
			input:    `-Da="one" -Db="two" -Dc="three"`,
			expected: []string{`-Da="one"`, `-Db="two"`, `-Dc="three"`},
		},
		{
			name:     "Newline in value - not special",
			input:    "-Dprop=value\n-Dother=val",
			expected: []string{"-Dprop=value\n-Dother=val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitArgsRespectingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParsePropertiesFromOpts(t *testing.T) {
	tests := []struct {
		name     string
		opts     string
		expected map[string]string
	}{
		{
			name:     "Single property",
			opts:     "-Dprop=value",
			expected: map[string]string{"prop": "value"},
		},
		{
			name:     "Multiple properties",
			opts:     "-Dprop1=value1 -Pprop2=value2",
			expected: map[string]string{"prop1": "value1", "prop2": "value2"},
		},
		{
			name:     "Empty string",
			opts:     "",
			expected: map[string]string{},
		},
		{
			name:     "Only JVM options",
			opts:     "-Xmx512m -Xms256m",
			expected: map[string]string{},
		},
		{
			name:     "Mixed JVM and properties",
			opts:     "-Xmx512m -Dprop=value -XX:+UseG1GC",
			expected: map[string]string{"prop": "value"},
		},
		{
			name:     "Property with URL value",
			opts:     "-PartifactoryUrl=http://localhost:8081",
			expected: map[string]string{"artifactoryUrl": "http://localhost:8081"},
		},
		{
			name:     "Whitespace handling",
			opts:     "  -Dprop=value  ",
			expected: map[string]string{"prop": "value"},
		},
		{
			name:     "Quoted value with space",
			opts:     `-Dprop="quoted value"`,
			expected: map[string]string{"prop": "quoted value"},
		},
		{
			name:     "Single quoted value with space",
			opts:     `-Dprop='single quoted'`,
			expected: map[string]string{"prop": "single quoted"},
		},
		{
			name:     "Multiple quoted properties",
			opts:     `-Da="value a" -Db="value b"`,
			expected: map[string]string{"a": "value a", "b": "value b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePropertiesFromOpts(tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPropertiesRegexPatterns(t *testing.T) {
	t.Run("propertiesFileRe - various formats", func(t *testing.T) {
		testCases := []struct {
			input    string
			hasMatch bool
		}{
			{"key=value", true},
			{"key:value", true},
			{"key = value", true},
			{"  key = value  ", true},
			{"# comment", false},
			{"", false},
			{"key=", true}, // Empty value is technically a match
		}

		for _, tc := range testCases {
			matches := propertiesFileRe.FindStringSubmatch(tc.input)
			if tc.hasMatch {
				assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			}
		}
	})

	t.Run("extBlockRe - property patterns", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected []string
		}{
			{`myProp = "value"`, []string{"myProp", "value"}},
			{`myProp = 'value'`, []string{"myProp", "value"}},
			{`my_prop = "value"`, []string{"my_prop", "value"}},
			{`my.prop = "value"`, []string{"my.prop", "value"}},
			{`_private = "value"`, []string{"_private", "value"}},
		}

		for _, tc := range testCases {
			matches := extBlockRe.FindStringSubmatch(tc.input)
			if len(tc.expected) > 0 {
				assert.NotNil(t, matches, "Expected match for: %s", tc.input)
				if matches != nil {
					assert.Equal(t, tc.expected[0], matches[1], "Key mismatch for: %s", tc.input)
					assert.Equal(t, tc.expected[1], matches[2], "Value mismatch for: %s", tc.input)
				}
			}
		}
	})

	t.Run("Property placeholder patterns", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{"${version}", "version"},
			{"${project.version}", "project.version"},
			{"${findProperty('key')}", "findProperty('key')"},
		}

		for _, tc := range testCases {
			matches := propPlaceHolderRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "Placeholder mismatch for: %s", tc.input)
			}
		}
	})

	t.Run("Simple variable patterns", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{"$version", "version"},
			{"$project.version", "project.version"},
			{"$my_var", "my_var"},
		}

		for _, tc := range testCases {
			matches := propVarRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "Variable mismatch for: %s", tc.input)
			}
		}
	})
}
