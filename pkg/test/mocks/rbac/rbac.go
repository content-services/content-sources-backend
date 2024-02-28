package rbac

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

const (
	RbacV1Access = "/api/rbac/v1/access/"
)

type RbacAccessRequest struct {
	Application string `query:"application"`
	Username    string `query:"username"`
	OrderBy     string `query:"order_by"`
	Limit       int    `query:"limit"`
	Offset      int    `query:"offset"`
}

type RbacMeta struct {
	Count  int `json:"count"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type RbacLinks struct {
	First    *string `json:"first"`
	Previous *string `json:"previous"`
	Next     *string `json:"next"`
	Last     *string `json:"last"`
}

type RbacData struct {
	Permission string `json:"permission,omitempty"`
}

type RbacAccessResponse struct {
	Meta  RbacMeta   `json:"meta"`
	Links RbacLinks  `json:"links,omitempty"`
	Data  []RbacData `json:"data"`
}

type CommonError struct {
	Detail string `json:"detail"`
	Source string `json:"source"`
	Status string `json:"status"`
}

type RbacErrorResponse struct {
	Errors []CommonError `json:"errors"`
}

func stringInSlice(match string, slice []string) bool {
	for _, item := range slice {
		if item == match {
			return true
		}
	}
	return false
}

func MockRbac(c echo.Context) error {
	var request RbacAccessRequest
	if err := c.Bind(&request); err != nil {
		c.Error(err)
		return err
	}

	const xRhIdentityHeader = "X-Rh-Identity"
	var (
		xRhIdentityB64  string
		xRhIdentityJson []byte
		xRhIdentity     identity.XRHID
		err             error
	)

	if xRhIdentityB64 = c.Request().Header.Get(xRhIdentityHeader); xRhIdentityB64 == "" {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("'%s' header cannot be empty", xRhIdentityHeader))
	}
	if xRhIdentityJson, err = b64.StdEncoding.DecodeString(xRhIdentityB64); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("'%s' wrong base64 format", xRhIdentityHeader))
	}
	if err = json.Unmarshal(xRhIdentityJson, &xRhIdentity); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("'%s' wrong json format", xRhIdentityHeader))
	}

	if xRhIdentity.Identity.OrgID == "" && xRhIdentity.Identity.Internal.OrgID != "" {
		xRhIdentity.Identity.OrgID = xRhIdentity.Identity.Internal.OrgID
	}

	linkData := c.Request().URL.Path + "?application=" + request.Application + "&offset=0&limit=1000"
	outputAllAllowed := RbacAccessResponse{
		Meta: RbacMeta{
			Count:  1,
			Limit:  1000,
			Offset: 0,
		},
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{
			{
				Permission: request.Application + ":*:*",
			},
		},
	}

	if request.Application != "content-sources" {
		log.Debug().Msgf("application rbac requested is not 'content-sources'")
		return c.JSON(http.StatusOK, outputAllAllowed)
	}

	outputEmpty := RbacAccessResponse{
		Meta: RbacMeta{
			Count:  0,
			Limit:  1000,
			Offset: 0,
		},
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{},
	}

	outputReadWrite := RbacAccessResponse{
		Meta: RbacMeta{
			Count:  2,
			Limit:  1000,
			Offset: 0,
		},
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{
			{
				Permission: "content-sources:*:read",
			},
			{
				Permission: "content-sources:*:write",
			},
			{
				Permission: "content-sources:repositories:read",
			},
			{
				Permission: "content-sources:repositories:write",
			},
		},
	}
	outputRead := RbacAccessResponse{
		Meta: RbacMeta{
			Count:  2,
			Limit:  1000,
			Offset: 0,
		},
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{
			{
				Permission: "content-sources:*:read",
			},
			{
				Permission: "content-sources:repositories:read",
			},
		},
	}

	// Expected error response from RBAC service
	outputError := RbacErrorResponse{
		Errors: []CommonError{
			{
				Detail: "No data found for principal with username 'non_existing_username_lksjfhskdfhl'.",
				Source: "detail",
				Status: "400",
			},
		},
	}

	mocksConfig := config.Get().Mocks
	accountsReadWrite := mocksConfig.Rbac.UserReadWrite
	accountsRead := mocksConfig.Rbac.UserRead
	accountsEmpty := mocksConfig.Rbac.UserNoPermissions

	username := ""
	// If user is empty replace struct with empty user
	if xRhIdentity.Identity.User == nil {
		if xRhIdentity.Identity.ServiceAccount != nil {
			username = xRhIdentity.Identity.ServiceAccount.Username
		}
	} else {
		username = xRhIdentity.Identity.User.Username
	}

	switch {
	case stringInSlice(username, accountsReadWrite):
		log.Debug().Msgf("returning permissions for read and write")
		return c.JSON(http.StatusOK, outputReadWrite)
	case stringInSlice(username, accountsRead):
		log.Debug().Msgf("returning permissions for only read")
		return c.JSON(http.StatusOK, outputRead)
	case stringInSlice(username, accountsEmpty):
		log.Debug().Msgf("returning empty permissions")
		return c.JSON(http.StatusOK, outputEmpty)
	default:
		log.Debug().Msgf("returning error response")
		return c.JSON(http.StatusBadRequest, outputError)
	}
}
