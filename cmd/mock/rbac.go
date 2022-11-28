package main

import (
	"fmt"
	"net/http"

	b64 "encoding/base64"
	"encoding/json"

	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

const (
	// FIXME Using the same access that into the Red Hat Insights doc
	RbacV1Access = "/api/rbac/v1/access/"
)

// FIXME Potentially duplicating structures; double check to import the necessary library instead of duplicate code
type RbacAccessRequest struct {
	Application string `query:"application"`
	Username    string `query:"username"`
	OrderBy     string `query:"order_by"`
	Limit       int    `query:"limit"`
	Offset      int    `query:"offset"`
}

type RbacMeta struct {
	Count int `json:"count"`
}

type RbacLinks struct {
	First    *string `json:"first"`
	Previous *string `json:"previous"`
	Next     *string `json:"next"`
	Last     *string `json:"last"`
}

type RbacData struct {
	Permission          string `json:"permission,omitempty"`
	ResourceDefinitions struct {
		AttributeFilter struct {
			Key       string `json:"key,omitempty"`
			Operation string `json:"operation,omitempty"`
			Value     string `json:"value,omitempty"`
		} `json:"attributeFilter,omitempty"`
	} `json:"resourceDefinitions,omitempty"`
}

type RbacAccessResponse struct {
	Meta  RbacMeta   `json:"meta"`
	Links RbacLinks  `json:"links,omitempty"`
	Data  []RbacData `json:"data"`
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
	outputAdmin := RbacAccessResponse{
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
		},
	}
	outputDefault := RbacAccessResponse{
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{
			{
				Permission: "content-sources:*:read",
			},
		},
	}
	outputEmpty := RbacAccessResponse{
		Links: RbacLinks{
			First: &linkData,
			Last:  &linkData,
		},
		Data: []RbacData{},
	}

	switch {
	case xRhIdentity.Identity.OrgID == "12345" && xRhIdentity.Identity.AccountNumber == "11111":
		return c.JSON(http.StatusOK, outputAdmin)
	case xRhIdentity.Identity.OrgID == "12345" && xRhIdentity.Identity.AccountNumber == "22222":
		return c.JSON(http.StatusOK, outputDefault)
	default:
		return c.JSON(http.StatusOK, outputEmpty)
	}
}
