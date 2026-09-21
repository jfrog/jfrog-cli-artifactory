"""
Tests for huggingface_upload.py's HfUriError-swallowing behavior.

Context: huggingface_hub >= 1.20.0 raises HfUriError from HfApi.upload_folder()
when parsing the commit response's URL (RepoUrl parsing, PR #4324 upstream).
Artifactory returns a placeholder commitUrl that this stricter parser rejects,
even though the files were already committed before the response is parsed.
upload() swallows that specific error - but only when its `uri` looks like the
full URL CommitInfo parses (contains "://"), never a bare repo_id/revision -
and treats the upload as successful. Every other exception must still
propagate, including HfUriError raised over a plain identifier.

huggingface_hub.errors.HfUriError already exists in huggingface_hub==1.19.0
(the version this repo's CI pins for HuggingFace-Tests) with the same
uri/msg constructor upload() relies on - it's PR #4324's new RepoUrl
call site that's version-gated, not the exception class itself. These
tests import and raise the real HfUriError rather than a stand-in, so they
exercise the exact exception type and attribute shape production code will
actually receive.
"""
import io
import unittest
from unittest.mock import patch

from huggingface_hub.errors import HfUriError

import huggingface_upload


class UploadHfUriErrorTest(unittest.TestCase):
    @patch("huggingface_upload.HfApi")
    def test_swallows_hf_uri_error_after_successful_upload(self, mock_hf_api_class):
        mock_hf_api_class.return_value.upload_folder.side_effect = HfUriError(
            uri="https://artifactory.example/placeholder", msg="could not parse"
        )

        # Must return cleanly (no exception) - the files were already committed,
        # only the response URL failed to parse.
        stderr = io.StringIO()
        with patch("sys.stderr", stderr):
            huggingface_upload.upload(
                folder_path="/tmp/folder", repo_id="org/repo", repo_type="model", revision="main"
            )

        # Swallowing must leave a trail on stderr, not fail silently.
        logged = stderr.getvalue()
        self.assertIn("swallowed HfUriError", logged)
        self.assertIn("org/repo", logged)

    @patch("huggingface_upload.HfApi")
    def test_propagates_hf_uri_error_over_a_plain_identifier(self, mock_hf_api_class):
        # A bare repo_id/revision-shaped uri (no "://") is not the known
        # post-commit commitUrl parsing failure - e.g. a malformed repo_id
        # rejected before any commit happened. Must not be swallowed.
        mock_hf_api_class.return_value.upload_folder.side_effect = HfUriError(
            uri="not-a-real-repo-id", msg="could not parse"
        )

        with self.assertRaises(HfUriError):
            huggingface_upload.upload(
                folder_path="/tmp/folder", repo_id="org/repo", repo_type="model", revision="main"
            )

    @patch("huggingface_upload.HfApi")
    def test_other_exceptions_still_propagate(self, mock_hf_api_class):
        mock_hf_api_class.return_value.upload_folder.side_effect = ValueError("network error")

        with self.assertRaises(ValueError):
            huggingface_upload.upload(
                folder_path="/tmp/folder", repo_id="org/repo", repo_type="model", revision="main"
            )


if __name__ == "__main__":
    unittest.main()
