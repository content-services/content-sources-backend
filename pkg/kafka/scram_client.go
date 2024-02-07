package kafka

import "github.com/xdg/scram"

// SCRAMClient implementation for the SCRAM authentication
type SCRAMClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

// Begin prepares the client for the SCRAM exchange
func (x *SCRAMClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

// Step steps client through the SCRAM exchange
func (x *SCRAMClient) Step(challenge string) (response string, err error) {
	response, err = x.ClientConversation.Step(challenge)
	return
}

// Done should return true when the SCRAM conversation
// is over.
func (x *SCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}
