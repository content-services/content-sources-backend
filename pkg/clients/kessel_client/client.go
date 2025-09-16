package kessel_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	url2 "net/url"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/project-kessel/kessel-sdk-go/kessel/auth"
	"github.com/project-kessel/kessel-sdk-go/kessel/inventory/v1beta2"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type KesselClient interface {
	GetRootWorkspaceID(ctx context.Context, orgID string) (string, int, error)
	CheckRead(ctx context.Context, workspaceID string, permission string) (bool, error)
	CheckWrite(ctx context.Context, workspaceID string, permission string) (bool, error)
}

type kesselClientImpl struct {
	timeout time.Duration
}

func NewKesselClient() (KesselClient, error) {
	if !config.KesselConfigured() {
		return nil, fmt.Errorf("kessel configuration not configured")
	}

	kesselConfig := config.Get().Clients.Kessel
	if kesselConfig.Auth.Enabled && (kesselConfig.Auth.ClientID == "" || kesselConfig.Auth.ClientSecret == "" || kesselConfig.Auth.OIDCIssuer == "") {
		return nil, fmt.Errorf("kessel authentication is enabled but not configured")
	}

	return &kesselClientImpl{timeout: kesselConfig.Timeout}, nil
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
		return "", statusCode, fmt.Errorf("kessel not configured")
	}

	rbacUrl, err := GetRbacURL()
	if err != nil {
		return "", statusCode, fmt.Errorf("failed to get rbac url: %w", err)
	}
	url := fmt.Sprintf("%s/api/rbac/v2/workspaces/?type=root", rbacUrl)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", statusCode, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Add("x-rh-rbac-org-id", orgID)

	if config.Get().Clients.Kessel.Auth.Enabled {
		log.Debug().Msgf("[Kessel] GetRootWorkspaceID: orgID %v", orgID)
		token, err := k.getToken(ctx)
		if err != nil {
			return "", statusCode, fmt.Errorf("error getting auth token: %w", err)
		}
		req.Header.Add("authorization", fmt.Sprintf("Bearer %s", token))
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

	object, subject, err := k.buildRequestObjects(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("error building reference objects: %w", err)
	}

	inventoryClient, conn, err := k.buildGRPCConnection()
	if err != nil {
		return false, fmt.Errorf("error building inventory client: %w", err)
	}
	defer conn.Close()

	req := &v1beta2.CheckRequest{
		Object:   object,
		Relation: permission,
		Subject:  subject,
	}

	log.Debug().Msgf("[Kessel] CheckRead: workspaceID %v", workspaceID)
	resp, err := inventoryClient.Check(reqCtx, req)
	if err != nil {
		return false, fmt.Errorf("kessel permission check failed: %w", err)
	}

	return resp.GetAllowed() == v1beta2.Allowed_ALLOWED_TRUE, nil
}

// CheckWrite checks if the user with the given workspaceID has the given permission
func (k *kesselClientImpl) CheckWrite(ctx context.Context, workspaceID string, permission string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	object, subject, err := k.buildRequestObjects(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("error building reference objects: %w", err)
	}

	inventoryClient, conn, err := k.buildGRPCConnection()
	if err != nil {
		return false, fmt.Errorf("error building inventory client: %w", err)
	}
	defer conn.Close()

	req := &v1beta2.CheckForUpdateRequest{
		Object:   object,
		Relation: permission,
		Subject:  subject,
	}

	log.Debug().Msgf("[Kessel] CheckWrite: workspaceID %v", workspaceID)
	resp, err := inventoryClient.CheckForUpdate(reqCtx, req)
	if err != nil {
		return false, fmt.Errorf("kessel permission check failed: %w", err)
	}

	return resp.GetAllowed() == v1beta2.Allowed_ALLOWED_TRUE, nil
}

// buildCheckRequest returns the objects need to make a CheckRequest or CheckForUpdateRequest call
func (k *kesselClientImpl) buildRequestObjects(ctx context.Context, workspaceID string) (object *v1beta2.ResourceReference, subject *v1beta2.SubjectReference, err error) {
	id := identity.GetIdentity(ctx)

	// Create the authorization check request
	object = &v1beta2.ResourceReference{
		ResourceType: "workspace",
		ResourceId:   workspaceID,
		Reporter: &v1beta2.ReporterReference{
			Type: "rbac",
		},
	}

	userID, err := extractUserID(id)
	if err != nil {
		return nil, nil, fmt.Errorf("error extracting user ID: %w", err)
	}

	subject = &v1beta2.SubjectReference{
		Resource: &v1beta2.ResourceReference{
			ResourceType: "principal",
			ResourceId:   fmt.Sprintf("redhat/%s", userID),
			Reporter: &v1beta2.ReporterReference{
				Type: "rbac",
			},
		},
	}

	return object, subject, nil
}

func (k *kesselClientImpl) buildGRPCConnection() (v1beta2.KesselInventoryServiceClient, *grpc.ClientConn, error) {
	kesselConfig := config.Get().Clients.Kessel
	kesselServer := strings.TrimPrefix(kesselConfig.Server, "http://")
	clientBuilder := v1beta2.NewClientBuilder(kesselServer)
	if !kesselConfig.Auth.Enabled || kesselConfig.Auth.GrpcInsecure {
		clientBuilder = clientBuilder.Insecure()
	} else {
		oauthCredentials := auth.NewOAuth2ClientCredentials(kesselConfig.Auth.ClientID, kesselConfig.Auth.ClientSecret, kesselConfig.Auth.OIDCIssuer)
		clientBuilder = clientBuilder.OAuth2ClientAuthenticated(&oauthCredentials, nil)
	}
	return clientBuilder.Build()
}

func (k *kesselClientImpl) getToken(ctx context.Context) (string, error) {
	kesselConfig := config.Get().Clients.Kessel
	if !kesselConfig.Auth.Enabled {
		return "", fmt.Errorf("auth not enabled")
	}
	oauthCredentials := auth.NewOAuth2ClientCredentials(kesselConfig.Auth.ClientID, kesselConfig.Auth.ClientSecret, kesselConfig.Auth.OIDCIssuer)
	resp, err := oauthCredentials.GetToken(ctx, auth.GetTokenOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting token: %w", err)
	}
	return resp.AccessToken, nil
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

func GetRbacURL() (string, error) {
	rbacBaseUrl := config.Get().Clients.RbacBaseUrl
	urlParsed, err := url2.ParseRequestURI(rbacBaseUrl)
	if err != nil {
		return "", fmt.Errorf("failed to parse rbac base url: %w", err)
	}
	return fmt.Sprintf("%s://%s", urlParsed.Scheme, urlParsed.Host), nil
}
