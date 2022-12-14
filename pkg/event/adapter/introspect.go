package adapter

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/event/message"
)

// IntroespectRequestPortIn scope the IntrospectRequestPortIn adapter
// so it will include any conversion to/from message.IntrospectRequestMessage
type IntrospectRequestPortIn interface {
	FromRepositoryResponse(repositoryResponse *api.RepositoryResponse) (*message.IntrospectRequestMessage, error)
	FromRepositoryRequest(repositoryRequest *api.RepositoryRequest, uuid string) (*message.IntrospectRequestMessage, error)
}

// IntrospectRequest implements IntrospectRequestPortIn
type IntrospectRequest struct{}

// Build a new IntrospectRequest adapter
func NewIntrospect() IntrospectRequest {
	return IntrospectRequest{}
}

// FromRepositoryResponse convert an api.RepositoryResponse into a message.IntrospectRequestMessage
// Return a message.IntrospectRequestMessage and nil error when everythin goes well, else
// a nil message and an error filled with the root cause.
func (a IntrospectRequest) FromRepositoryResponse(repositoryResponse *api.RepositoryResponse) (*message.IntrospectRequestMessage, error) {
	if repositoryResponse == nil {
		return nil, fmt.Errorf("repositoryResponse cannot be nil")
	}
	output := &message.IntrospectRequestMessage{
		Uuid: repositoryResponse.UUID,
		Url:  repositoryResponse.URL,
	}
	return output, nil
}

// FromRepositoryRequest convert an api.RepositoryRequest plus an uuid into a message.IntrospectRequestMessage
// Return a message.IntrospectRequestMessage and nil error when everythin goes well, else
// a nil message and an error filled with the root cause.
func (a IntrospectRequest) FromRepositoryRequest(repositoryRequest *api.RepositoryRequest, uuid string) (*message.IntrospectRequestMessage, error) {
	if repositoryRequest == nil {
		return nil, fmt.Errorf("repositoryRequest cannot be nil")
	}
	if repositoryRequest.URL == nil {
		return nil, fmt.Errorf("repositoryRequest.UUID or repositoryRequest.URL are nil")
	}
	if uuid == "" {
		return nil, fmt.Errorf("uuid cannot be empty")
	}
	output := &message.IntrospectRequestMessage{
		Uuid: uuid,
		Url:  *repositoryRequest.URL,
	}
	return output, nil
}
