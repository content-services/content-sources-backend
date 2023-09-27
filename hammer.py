import requests

def sendReq():
  url = "http://localhost:8000/api/content-sources/v1.0/repositories/6f202e47-a1d1-4813-8fc9-d506ef67cf7d/introspect/"
  headers = { "x-rh-identity":"eyJpZGVudGl0eSI6eyJ0eXBlIjoiVXNlciIsInVzZXIiOnsidXNlcm5hbWUiOiJqZG9lIn0sImludGVybmFsIjp7Im9yZ19pZCI6IjEyMyJ9fX0K" }
  x = requests.post(url, headers = headers)

for i in range(100):
  sendReq()

