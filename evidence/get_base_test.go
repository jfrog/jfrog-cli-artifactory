package evidence

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportEvidenceToFile(t *testing.T) {
	tests := []struct {
		name           string
		evidence       []byte
		outputFileName string
		format         string
		expectedError  bool
	}{
		{
			name:           "Valid JSON evidence with json output file name",
			evidence:       []byte(`{"key": "value"}`),
			outputFileName: "test_output.json",
			format:         "json",
			expectedError:  false,
		},
		{
			name:           "Valid JSON evidence with empty output file name",
			evidence:       []byte(`{"key": "value"}`),
			outputFileName: "test_output.json",
			format:         "",
			expectedError:  false,
		},
		{
			name:           "unsupported format",
			evidence:       []byte(`{"key": "value"}`),
			outputFileName: "test_output.json",
			format:         "unsupported",
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		// Create an instance of getEvidenceBase
		g := &getEvidenceBase{}

		// Call the method
		err := g.exportEvidenceToFile(tt.evidence, tt.outputFileName, tt.format)

		// Check for expected errors
		if tt.expectedError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			// Check that the output file is created and contains the expected content
			content, err := os.ReadFile("test_output.json")
			if err != nil {
				t.Errorf("Error reading output file: %v", err)
			}
			assert.Equal(t, tt.evidence, content)
		}
		// Clean up the created file if it was created
		if !tt.expectedError && tt.outputFileName != "" {
			err = os.Remove(tt.outputFileName)
			if err != nil {
				t.Errorf("Error removing output file: %v", err)
			}
		}
	}
}
