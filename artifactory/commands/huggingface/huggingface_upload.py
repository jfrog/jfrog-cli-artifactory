"""
HuggingFace Hub model and dataset upload utility.

This module provides functionality to upload models and datasets to HuggingFace Hub
using HfApi with configurable parameters.
"""

from huggingface_hub import HfApi


def upload(folder_path, repo_id, repo_type, revision=None, **kwargs):
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
    api.upload_folder(
        folder_path=folder_path,
        repo_id=repo_id,
        revision=revision,
        repo_type=repo_type,
        **kwargs
    )