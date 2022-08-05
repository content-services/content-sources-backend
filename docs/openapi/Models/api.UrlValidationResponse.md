# api.UrlValidationResponse
## Properties

| Name | Type | Description | Notes |
|------------ | ------------- | ------------- | -------------|
| **error** | **String** | Error message if the attribute is not valid | [optional] [default to null] |
| **http\_code** | **Integer** | If the metadata cannot be fetched successfully, the http code that is returned if the http request was completed | [optional] [default to null] |
| **metadata\_present** | **Boolean** | True if the metadata can be fetched successfully | [optional] [default to null] |
| **skipped** | **Boolean** | Skipped if the URL is not passed in for validation | [optional] [default to null] |
| **valid** | **Boolean** | Valid if not skipped and provided attribute is valid to be saved | [optional] [default to null] |

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

