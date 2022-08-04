# RepositoriesApi

All URIs are relative to *https://api.example.com/api/content_sources/v1.0*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**bulkCreateRepositories**](RepositoriesApi.md#bulkCreateRepositories) | **POST** /repositories/bulk_create/ | Bulk create repositories |
| [**createRepository**](RepositoriesApi.md#createRepository) | **POST** /repositories/ | Create Repository |
| [**deleteRepository**](RepositoriesApi.md#deleteRepository) | **DELETE** /repositories/{uuid} | Delete a repository |
| [**fullUpdateRepository**](RepositoriesApi.md#fullUpdateRepository) | **PUT** /repositories/{uuid} | Update Repository |
| [**getRepository**](RepositoriesApi.md#getRepository) | **GET** /repositories/{uuid} | Get Repository |
| [**listRepositories**](RepositoriesApi.md#listRepositories) | **GET** /repositories/ | List Repositories |
| [**listRepositoriesRpms**](RepositoriesApi.md#listRepositoriesRpms) | **GET** /repositories/:uuid/rpms | List Repositories RPMs |
| [**listRepositoryParameters**](RepositoriesApi.md#listRepositoryParameters) | **GET** /repository_parameters/ | List Repository Parameters |
| [**partialUpdateRepository**](RepositoriesApi.md#partialUpdateRepository) | **PATCH** /repositories/{uuid} | Partial Update Repository |
| [**searchRpm**](RepositoriesApi.md#searchRpm) | **POST** /rpms/names | Search RPMs |


<a name="bulkCreateRepositories"></a>
# **bulkCreateRepositories**
> List bulkCreateRepositories(api.RepositoryRequest)

Bulk create repositories

    bulk create repositories

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **api.RepositoryRequest** | [**List**](../Models/api.RepositoryRequest.md)| request body | |

### Return type

[**List**](../Models/api.RepositoryBulkCreateResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

<a name="createRepository"></a>
# **createRepository**
> api.RepositoryResponse createRepository(api.RepositoryRequest)

Create Repository

    create a repository

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **api.RepositoryRequest** | [**api.RepositoryRequest**](../Models/api.RepositoryRequest.md)| request body | |

### Return type

[**api.RepositoryResponse**](../Models/api.RepositoryResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

<a name="deleteRepository"></a>
# **deleteRepository**
> String deleteRepository(uuid)

Delete a repository

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **uuid** | **String**| Identifier of the Repository | [default to null] |

### Return type

**String**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="fullUpdateRepository"></a>
# **fullUpdateRepository**
> String fullUpdateRepository(uuid, api.RepositoryRequest)

Update Repository

    Fully update a repository

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **uuid** | **String**| Identifier of the Repository | [default to null] |
| **api.RepositoryRequest** | [**api.RepositoryRequest**](../Models/api.RepositoryRequest.md)| request body | |

### Return type

**String**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

<a name="getRepository"></a>
# **getRepository**
> api.RepositoryResponse getRepository(uuid)

Get Repository

    Get information about a Repository

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **uuid** | **String**| Identifier of the Repository | [default to null] |

### Return type

[**api.RepositoryResponse**](../Models/api.RepositoryResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="listRepositories"></a>
# **listRepositories**
> api.RepositoryCollectionResponse listRepositories()

List Repositories

    get repositories

### Parameters
This endpoint does not need any parameter.

### Return type

[**api.RepositoryCollectionResponse**](../Models/api.RepositoryCollectionResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

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

<a name="listRepositoryParameters"></a>
# **listRepositoryParameters**
> api.RepositoryParameterResponse listRepositoryParameters()

List Repository Parameters

    get repository parameters (Versions and Architectures)

### Parameters
This endpoint does not need any parameter.

### Return type

[**api.RepositoryParameterResponse**](../Models/api.RepositoryParameterResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="partialUpdateRepository"></a>
# **partialUpdateRepository**
> String partialUpdateRepository(uuid, api.RepositoryRequest)

Partial Update Repository

    Partially Update a repository

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **uuid** | **String**| Identifier of the Repository | [default to null] |
| **api.RepositoryRequest** | [**api.RepositoryRequest**](../Models/api.RepositoryRequest.md)| request body | |

### Return type

**String**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
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

