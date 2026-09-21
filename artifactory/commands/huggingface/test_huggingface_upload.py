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

huggingface_hub.errors.HfUriError may not exist in whatever huggingface_hub
version happens to be installed wherever this test runs (it's a recent
addition). Patching it with create=True injects a stand-in at the exact
import path upload()'s except block reads from, so this test exercises the
real isinstance() check in huggingface_upload.py regardless of the installed
huggingface_hub version - it is not a copy of the production logic.
"""
import io
import unittest
from unittest.mock import patch

import huggingface_upload


class FakeHfUriError(ValueError):
    """Stand-in for huggingface_hub.errors.HfUriError (same constructor shape)."""

    def __init__(self, uri, msg):
        self.uri = uri
        self.msg = msg
        super().__init__(f"Invalid HF URI '{uri}'. {msg}")


class UploadHfUriErrorTest(unittest.TestCase):
    @patch("huggingface_hub.errors.HfUriError", FakeHfUriError, create=True)
    @patch("huggingface_upload.HfApi")
    def test_swallows_hf_uri_error_after_successful_upload(self, mock_hf_api_class):
        mock_hf_api_class.return_value.upload_folder.side_effect = FakeHfUriError(
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

    @patch("huggingface_hub.errors.HfUriError", FakeHfUriError, create=True)
    @patch("huggingface_upload.HfApi")
    def test_propagates_hf_uri_error_over_a_plain_identifier(self, mock_hf_api_class):
        # A bare repo_id/revision-shaped uri (no "://") is not the known
        # post-commit commitUrl parsing failure - e.g. a malformed repo_id
        # rejected before any commit happened. Must not be swallowed.
        mock_hf_api_class.return_value.upload_folder.side_effect = FakeHfUriError(
            uri="not-a-real-repo-id", msg="could not parse"
        )

        with self.assertRaises(FakeHfUriError):
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
