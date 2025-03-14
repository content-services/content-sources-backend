package pulp_client

import (
	"context"
	"net/http"
	"os"

	zest "github.com/content-services/zest/release/v2025"
)

// CreateUpload Creates an upload
func (r *pulpDaoImpl) CreateUpload(ctx context.Context, size int64) (*zest.UploadResponse, int, error) {
	ctx, client := getZestClient(ctx)
	_, err := r.LookupOrCreateDomain(ctx, r.domainName)
	if err != nil {
		return nil, 0, err
	}

	statusCode := http.StatusInternalServerError
	upload := zest.Upload{}
	upload.Size = size
	readResp, httpResp, err := client.UploadsAPI.UploadsCreate(ctx, r.domainName).Upload(upload).Execute()
	if httpResp != nil {
		statusCode = httpResp.StatusCode
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, statusCode, errorWithResponseBody("creating upload", httpResp, err)
	}
	return readResp, statusCode, nil
}

// UploadChunk Uploads a chunk for an upload
func (r *pulpDaoImpl) UploadChunk(ctx context.Context, uploadHref string, contentRange string, file *os.File, sha256 string) (*zest.UploadResponse, int, error) {
	ctx, client := getZestClient(ctx)
	statusCode := http.StatusInternalServerError

	readResp, httpResp, err := client.UploadsAPI.UploadsUpdate(ctx, uploadHref).ContentRange(contentRange).File(file).Sha256(sha256).Execute()
	if httpResp != nil {
		statusCode = httpResp.StatusCode
		defer httpResp.Body.Close()
	}
	if err != nil {
		return &zest.UploadResponse{}, statusCode, errorWithResponseBody("uploading file chunk", httpResp, err)
	}
	return readResp, statusCode, nil
}

// FinishUpload Finishes an upload
func (r *pulpDaoImpl) FinishUpload(ctx context.Context, uploadHref string, sha256 string) (*zest.AsyncOperationResponse, int, error) {
	ctx, client := getZestClient(ctx)
	uploadCommit := zest.UploadCommit{}
	uploadCommit.Sha256 = sha256
	statusCode := http.StatusInternalServerError

	readResp, httpResp, err := client.UploadsAPI.UploadsCommit(ctx, uploadHref).UploadCommit(uploadCommit).Execute()
	if httpResp != nil {
		statusCode = httpResp.StatusCode
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, statusCode, errorWithResponseBody("finishing upload", httpResp, err)
	}
	return readResp, statusCode, nil
}
