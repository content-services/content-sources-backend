package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	// FIXME Using the same access that into the Red Hat Insights doc
	RbacV1Access = "/api/rbac/v1/access/"
)

/*
	{
	  "meta": {
	    "count": 30
	  },
	  "links": {
	    "first": "/api/v1/(resources)/?offset=0&limit=10",
	    "previous": "/api/v1/(resources)/?offset=20&limit=10",
	    "next": "/api/v1/(resources)/?offset=40&limit=10",
	    "last": "/api/v1/(resources)/?offset=90&limit=10"
	  },
	  "data": [
	    {
	      "permission": "cost-management:*:read",
	      "resourceDefinitions": [
	        {
	          "attributeFilter": {
	            "key": "cost-management.aws.account",
	            "operation": "equal",
	            "value": "123456"
	          }
	        }
	      ]
	    }
	  ]
	}
*/
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
	First    string `json:"first,omitempty"`
	Previous string `json:"previous,omitempty"`
	Next     string `json:"next,omitempty"`
	Last     string `json:"last,omitempty"`
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
	output := RbacAccessResponse{
		Data: []RbacData{
			{
				Permission: "content-sources:*:read",
			},
			{
				Permission: "content-sources:*:write",
			},
		},
	}
	output.Meta.Count = len(output.Data)
	return c.JSON(http.StatusOK, output)
}
