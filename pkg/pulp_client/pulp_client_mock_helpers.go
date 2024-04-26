package pulp_client

func (m *MockPulpClient) WithDomainMock() *MockPulpClient {
	m.On("WithDomain", "").Return(m)
	return m
}
