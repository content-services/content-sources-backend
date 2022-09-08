# RepositoriesApi

All URIs are relative to *https://api.example.com/api/content-sources/v1.0*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**bulkCreateRepositories**](RepositoriesApi.md#bulkCreateRepositories) | **POST** /repositories/bulk_create/ | Bulk create repositories |
| [**createRepository**](RepositoriesApi.md#createRepository) | **POST** /repositories/ | Create Repository |
| [**deleteRepository**](RepositoriesApi.md#deleteRepository) | **DELETE** /repositories/{uuid} | Delete a repository |
| [**fullUpdateRepository**](RepositoriesApi.md#fullUpdateRepository) | **PUT** /repositories/{uuid} | Update Repository |
| [**getRepository**](RepositoriesApi.md#getRepository) | **GET** /repositories/{uuid} | Get Repository |
| [**listRepositories**](RepositoriesApi.md#listRepositories) | **GET** /repositories/ | List Repositories |
| [**listRepositoriesRpms**](RepositoriesApi.md#listRepositoriesRpms) | **GET** /repositories/{uuid}/rpms | List Repositories RPMs |
| [**listRepositoryParameters**](RepositoriesApi.md#listRepositoryParameters) | **GET** /repository_parameters/ | List Repository Parameters |
| [**partialUpdateRepository**](RepositoriesApi.md#partialUpdateRepository) | **PATCH** /repositories/{uuid} | Partial Update Repository |
| [**searchRpm**](RepositoriesApi.md#searchRpm) | **POST** /rpms/names | Search RPMs |
| [**validateRepositoryParameters**](RepositoriesApi.md#validateRepositoryParameters) | **POST** /repository_parameters/validate/ | Validate parameters prior to creating a repository |


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
> api.RepositoryCollectionResponse listRepositories(offset, limit, version, arch, available\_for\_version, available\_for\_arch, search)

List Repositories

    list repositories

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **offset** | **Integer**| Offset into the list of results to return in the response | [optional] [default to null] |
| **limit** | **Integer**| Limit the number of items returned | [optional] [default to null] |
| **version** | **String**| Comma separated list of architecture to optionally filter-on (e.g. &#39;x86_64,s390x&#39; would return Repositories with x86_64 or s390x only) | [optional] [default to null] |
| **arch** | **String**| Comma separated list of versions to optionally filter-on  (e.g. &#39;7,8&#39; would return Repositories with versions 7 or 8 only) | [optional] [default to null] |
| **available\_for\_version** | **String**| Filter by compatible arch (e.g. &#39;x86_64&#39; would return Repositories with the &#39;x86_64&#39; arch and Repositories where arch is not set) | [optional] [default to null] |
| **available\_for\_arch** | **String**| Filter by compatible version (e.g. 7 would return Repositories with the version 7 or where version is not set) | [optional] [default to null] |
| **search** | **String**| Search term for name and url. | [optional] [default to null] |

### Return type

[**api.RepositoryCollectionResponse**](../Models/api.RepositoryCollectionResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

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

<a name="validateRepositoryParameters"></a>
# **validateRepositoryParameters**
> List validateRepositoryParameters(api.RepositoryValidationRequest)

Validate parameters prior to creating a repository

    Validate parameters prior to creating a repository, including checking if remote yum metadata is present

### Parameters

|Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **api.RepositoryValidationRequest** | [**List**](../Models/api.RepositoryValidationRequest.md)| request body | |

### Return type

[**List**](../Models/api.RepositoryValidationResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

