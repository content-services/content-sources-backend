package pulp_client

import "context"

func (m *MockPulpClient) WithContextMock() *MockPulpClient {
	m.On("WithContext", context.Background()).Return(m)
	return m
}

func (m *MockPulpClient) WithDomainMock() *MockPulpClient {
	m.On("WithDomain", "").Return(m)
	return m
}
