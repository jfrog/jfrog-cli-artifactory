"""
HuggingFace Hub model and dataset download utility.

This module provides functionality to download models and datasets from HuggingFace Hub
using snapshot_download with configurable parameters.
"""

from huggingface_hub import snapshot_download


def download(repo_id, repo_type, etag_timeout, revision=None,  **kwargs):
    """
    Download a model or dataset from HuggingFace Hub.
    Args:
        repo_id (str): The repository ID (e.g., "username/model-name" or "username/dataset-name").
        revision (str, optional): The specific revision/branch/tag to download.
                                  Defaults to None (main branch).
        repo_type (str, optional): Type of repository. Defaults to "model".
                                   Can be "model", "dataset", or "space".
        etag_timeout (int, optional): Timeout in seconds for ETag validation.
                                       Defaults to 86400 (24 hours).
        **kwargs: Additional arguments to pass to snapshot_download.
    Returns:
        str: Path to the downloaded model or dataset directory.
    Example:
        >>> download("bert-base-uncased", revision="main")
        '/path/to/downloaded/model'
        >>> download("username/dataset-name", repo_type="dataset")
        '/path/to/downloaded/dataset'
    """
    return snapshot_download(
        repo_id=repo_id,
        revision=revision,
        repo_type=repo_type,
        etag_timeout=etag_timeout,
        **kwargs
    )