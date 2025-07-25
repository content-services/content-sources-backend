package kessel_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	kesselAPIv2 "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta2"
	"github.com/project-kessel/inventory-client-go/common"
	kesselClientV2 "github.com/project-kessel/inventory-client-go/v1beta2"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"google.golang.org/grpc"
)

type KesselClient interface {
	GetRootWorkspaceID(ctx context.Context, orgID string) (string, int, error)
	CheckRead(ctx context.Context, workspaceID string, permission string) (bool, error)
	CheckWrite(ctx context.Context, workspaceID string, permission string) (bool, error)
}

type kesselClientImpl struct {
	client      *kesselClientV2.InventoryClient
	timeout     time.Duration
	tokenClient *common.TokenClient
}

func NewKesselClient() (KesselClient, error) {
	if !config.KesselConfigured() {
		return nil, fmt.Errorf("kessel configuration not configured")
	}

	kesselConfig := config.Get().Clients.Kessel

	// Create Kessel client configuration options
	options := []func(*common.Config){
		common.WithgRPCUrl(strings.TrimPrefix(kesselConfig.Server, "http://")),
		common.WithTLSInsecure(kesselConfig.Insecure),
	}

	var tokenClient *common.TokenClient
	if kesselConfig.Auth.Enabled {
		options = append(options, common.WithAuthEnabled(kesselConfig.Auth.ClientID, kesselConfig.Auth.ClientSecret, kesselConfig.Auth.OIDCIssuer))
	}

	// Create Kessel configuration
	cfg := common.NewConfig(options...)

	// Create Kessel client
	client, err := kesselClientV2.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kessel client: %w", err)
	}

	// Create token client if auth is enabled
	if kesselConfig.Auth.Enabled {
		tokenClient = common.NewTokenClient(cfg)
	}

	return &kesselClientImpl{
		client:      client,
		timeout:     kesselConfig.Timeout,
		tokenClient: tokenClient,
	}, nil
}

type WorkspaceResponse struct {
	Data []Workspace `json:"data"`
}
type Workspace struct {
	ID string `json:"id"`
}

func (k *kesselClientImpl) GetRootWorkspaceID(ctx context.Context, orgID string) (string, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	if !config.KesselConfigured() {
		return "", 0, nil
	}

	server := config.Get().Clients.RbacUrl
	url := fmt.Sprintf("%s/api/rbac/v2/workspaces/?type=root", server)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", statusCode, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Add("x-rh-rbac-org-id", orgID)

	// Add auth token if available
	if k.tokenClient != nil {
		token, err := k.tokenClient.GetToken()
		if err != nil {
			return "", statusCode, fmt.Errorf("error obtaining authentication token: %w", err)
		}
		req.Header.Add("authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	}

	client := &http.Client{Timeout: k.timeout}
	var body []byte
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", statusCode, fmt.Errorf("error during read response body: %w", err)
		}

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}
	}
	if err != nil {
		return "", statusCode, fmt.Errorf("error during GET request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return "", statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	var response WorkspaceResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", statusCode, fmt.Errorf("error unmarshalling response: %w", err)
	}

	if len(response.Data) != 1 {
		return "", statusCode, fmt.Errorf("unexpected number of default workspaces: %d", len(response.Data))
	}

	return response.Data[0].ID, statusCode, nil
}

// CheckRead checks if the user with the given workspaceID has the given permission
func (k *kesselClientImpl) CheckRead(ctx context.Context, workspaceID string, permission string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	object, subject, opts, err := k.buildRequestObjects(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("error building reference objects: %w", err)
	}

	req := &kesselAPIv2.CheckRequest{
		Object:   object,
		Relation: permission,
		Subject:  subject,
	}

	resp, err := k.client.KesselInventoryService.Check(reqCtx, req, opts...)
	if err != nil {
		return false, fmt.Errorf("kessel permission check failed: %w", err)
	}

	return resp.GetAllowed() == kesselAPIv2.Allowed_ALLOWED_TRUE, nil
}

// CheckWrite checks if the user with the given workspaceID has the given permission
func (k *kesselClientImpl) CheckWrite(ctx context.Context, workspaceID string, permission string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	object, subject, opts, err := k.buildRequestObjects(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("error building reference objects: %w", err)
	}

	req := &kesselAPIv2.CheckForUpdateRequest{
		Object:   object,
		Relation: permission,
		Subject:  subject,
	}

	resp, err := k.client.KesselInventoryService.CheckForUpdate(reqCtx, req, opts...)
	if err != nil {
		return false, fmt.Errorf("kessel permission check failed: %w", err)
	}

	return resp.GetAllowed() == kesselAPIv2.Allowed_ALLOWED_TRUE, nil
}

// buildCheckRequest returns the objects need to make a CheckRequest or CheckForUpdateRequest call
func (k *kesselClientImpl) buildRequestObjects(ctx context.Context, workspaceID string) (object *kesselAPIv2.ResourceReference, subject *kesselAPIv2.SubjectReference, options []grpc.CallOption, err error) {
	id := identity.GetIdentity(ctx)

	// Create the authorization check request
	object = &kesselAPIv2.ResourceReference{
		ResourceType: "workspace",
		ResourceId:   workspaceID,
		Reporter: &kesselAPIv2.ReporterReference{
			Type: "rbac",
		},
	}

	userID, err := extractUserID(id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error extracting user ID: %w", err)
	}

	subject = &kesselAPIv2.SubjectReference{
		Resource: &kesselAPIv2.ResourceReference{
			ResourceType: "principal",
			ResourceId:   fmt.Sprintf("redhat/%s", userID),
			Reporter: &kesselAPIv2.ReporterReference{
				Type: "rbac",
			},
		},
	}

	// Get call options for authentication if enabled
	var opts []grpc.CallOption
	if k.tokenClient != nil {
		opts, err = k.client.GetTokenCallOption()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get token call option: %w", err)
		}
	}

	return object, subject, opts, nil
}

func extractUserID(identity identity.XRHID) (string, error) {
	switch identity.Identity.Type {
	case "User":
		return identity.Identity.User.UserID, nil
	case "ServiceAccount":
		return identity.Identity.ServiceAccount.UserId, nil
	default:
		return "", fmt.Errorf("unknown identity type: %s", identity.Identity.Type)
	}
}
