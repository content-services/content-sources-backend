# RpmsApi

All URIs are relative to *https://api.example.com/api/content-sources/v1.0*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**listRepositoriesRpms**](RpmsApi.md#listRepositoriesRpms) | **GET** /repositories/{uuid}/rpms | List Repositories RPMs |
| [**searchRpm**](RpmsApi.md#searchRpm) | **POST** /rpms/names | Search RPMs |


<a name="listRepositoriesRpms"></a>
# **listRepositoriesRpms**
> api.RepositoryRpmCollectionResponse listRepositoriesRpms(uuid)

List Repositories RPMs

    list repositories RPMs

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **uuid** | **String**| Identifier of the Repository | [default to null] |

### Return type

[**api.RepositoryRpmCollectionResponse**](../Models/api.RepositoryRpmCollectionResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="searchRpm"></a>
# **searchRpm**
> List searchRpm(api.SearchRpmRequest)

Search RPMs

    Search RPMs for a given list of repository URLs

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **api.SearchRpmRequest** | [**api.SearchRpmRequest**](../Models/api.SearchRpmRequest.md)| request body | |

### Return type

[**List**](../Models/api.SearchRpmResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

