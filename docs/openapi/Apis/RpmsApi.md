# RpmsApi

All URIs are relative to *https://api.example.com/api/content_sources/v1.0*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**listRepositoriesRpms**](RpmsApi.md#listRepositoriesRpms) | **GET** /repositories/:uuid/rpms | List Repositories RPMs |
| [**searchRpm**](RpmsApi.md#searchRpm) | **POST** /rpms/names | Search RPMs |


<a name="listRepositoriesRpms"></a>
# **listRepositoriesRpms**
> api.RepositoryRpmCollectionResponse listRepositoriesRpms()

List Repositories RPMs

    get repositories RPMs

### Parameters
This endpoint does not need any parameter.

### Return type

[**api.RepositoryRpmCollectionResponse**](../Models/api.RepositoryRpmCollectionResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="searchRpm"></a>
# **searchRpm**
> api.SearchRpmRequest searchRpm()

Search RPMs

    Search RPMs for a given list of repository URLs

### Parameters
This endpoint does not need any parameter.

### Return type

[**api.SearchRpmRequest**](../Models/api.SearchRpmRequest.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

