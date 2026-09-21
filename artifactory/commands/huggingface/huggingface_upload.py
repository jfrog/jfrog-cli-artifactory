"""
HuggingFace Hub model and dataset upload utility.

This module provides functionality to upload models and datasets to HuggingFace Hub
using HfApi with configurable parameters.
"""

import sys

from huggingface_hub import HfApi


def upload(folder_path, repo_id, repo_type, revision, **kwargs):
    """
    Upload a model or dataset folder to HuggingFace Hub.

    Args:
        folder_path (str): Path to the folder to upload.
        repo_id (str): The repository ID (e.g., "username/model-name" or "username/dataset-name").
        revision (str, optional): The specific revision/branch/tag to upload to.
                                  Defaults to None (main branch).
        repo_type (str, optional): Type of repository. Defaults to "model".
                                   Can be "model", "dataset".
        **kwargs: Additional arguments to pass to upload_folder.

    Example:
        >>> upload_model("/path/to/model", "username/my-model", revision="main")
        >>> upload_model("/path/to/dataset", "username/dataset-name", repo_type="dataset")
    """
    api = HfApi()
    try:
        api.upload_folder(
            folder_path=folder_path,
            repo_id=repo_id,
            revision=revision,
            repo_type=repo_type,
            **kwargs
        )
    except Exception as e:
        # huggingface_hub >= 1.20.0 uses strict parse_hf_uri inside RepoUrl (PR #4324).
        # Artifactory returns a placeholder commitUrl that fails this parser, but the
        # files were already committed before the response is parsed. Swallow the URI
        # parse error so callers treat the upload as successful.
        try:
            from huggingface_hub.errors import HfUriError
        except ImportError:
            HfUriError = None
        if HfUriError is None or not isinstance(e, HfUriError):
            raise
        # HfUriError is only known to be raised here via CommitInfo parsing the
        # post-commit commitUrl (always a full URL). A bare repo_id/revision string
        # can never contain "://", so if some other call path ever raises HfUriError
        # over a plain identifier, that's a real failure (e.g. a malformed repo_id)
        # and must not be swallowed just because the exception type matches.
        uri = getattr(e, "uri", "") or ""
        if "://" not in uri:
            raise
        # Not silent: this is a real behavior change (an upload failure becomes a
        # success), so anyone debugging a "phantom" success needs a trail. Printed to
        # stderr, which the Go caller already streams through unmodified (see
        # huggingFaceUpload.go's cmd.Stderr), so it won't affect the stdout JSON
        # success/failure contract the caller actually parses.
        print(
            f"jf hf upload: swallowed HfUriError while parsing the commit response for "
            f"'{repo_id}' - files were already uploaded successfully; only the response "
            f"URL failed to parse ({e})",
            file=sys.stderr,
        )
        return