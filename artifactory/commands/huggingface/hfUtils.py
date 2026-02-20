import os
from huggingface_download import download

# 1. Configure JFrog Artifactory as HuggingFace ML endpoint
# Format: https://<server>/artifactory/api/huggingfaceml/<repo-key>

# 2. Set authentication token (if required)
# os.environ["HF_TOKEN"] = "your-jfrog-access-token"

# 3. Define your parameters (from screenshot)
MY_REPO = "admin"          # Repo ID from path: models/admin/
MY_TYPE = "model"          # Repository type
MY_REVISION = "main"       # Use branch name, not the timestamp folder
MY_ETAG_TIMEOUT = 86400    # ETag timeout in seconds (24 hours)

# 4. Call the function
print(f"Starting download of {MY_REPO} (revision: {MY_REVISION})...")
print(f"From endpoint: {os.environ['HF_ENDPOINT']}")

model_path = download(
    repo_id=MY_REPO,
    repo_type=MY_TYPE,
    etag_timeout=MY_ETAG_TIMEOUT,
    revision=MY_REVISION
)

print(f"Download complete! Model saved to: {model_path}")