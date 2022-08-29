# Documentation for ContentSourcesBackend

<a name="documentation-for-api-endpoints"></a>
## Documentation for API Endpoints

All URIs are relative to *https://api.example.com/api/content_sources/v1.0*

| Class | Method | HTTP request | Description |
|------------ | ------------- | ------------- | -------------|
| *RepositoriesApi* | [**bulkCreateRepositories**](Apis/RepositoriesApi.md#bulkcreaterepositories) | **POST** /repositories/bulk_create/ | Bulk create repositories |
*RepositoriesApi* | [**createRepository**](Apis/RepositoriesApi.md#createrepository) | **POST** /repositories/ | Create Repository |
*RepositoriesApi* | [**deleteRepository**](Apis/RepositoriesApi.md#deleterepository) | **DELETE** /repositories/{uuid} | Delete a repository |
*RepositoriesApi* | [**fullUpdateRepository**](Apis/RepositoriesApi.md#fullupdaterepository) | **PUT** /repositories/{uuid} | Update Repository |
*RepositoriesApi* | [**getRepository**](Apis/RepositoriesApi.md#getrepository) | **GET** /repositories/{uuid} | Get Repository |
*RepositoriesApi* | [**listRepositories**](Apis/RepositoriesApi.md#listrepositories) | **GET** /repositories/ | List Repositories |
*RepositoriesApi* | [**listRepositoriesRpms**](Apis/RepositoriesApi.md#listrepositoriesrpms) | **GET** /repositories/{uuid}/rpms | List Repositories RPMs |
*RepositoriesApi* | [**listRepositoryParameters**](Apis/RepositoriesApi.md#listrepositoryparameters) | **GET** /repository_parameters/ | List Repository Parameters |
*RepositoriesApi* | [**partialUpdateRepository**](Apis/RepositoriesApi.md#partialupdaterepository) | **PATCH** /repositories/{uuid} | Partial Update Repository |
*RepositoriesApi* | [**searchRpm**](Apis/RepositoriesApi.md#searchrpm) | **POST** /rpms/names | Search RPMs |
*RepositoriesApi* | [**validateRepositoryParameters**](Apis/RepositoriesApi.md#validaterepositoryparameters) | **POST** /repository_parameters/validate/ | Validate parameters prior to creating a repository |
| *RpmsApi* | [**listRepositoriesRpms**](Apis/RpmsApi.md#listrepositoriesrpms) | **GET** /repositories/{uuid}/rpms | List Repositories RPMs |
*RpmsApi* | [**searchRpm**](Apis/RpmsApi.md#searchrpm) | **POST** /rpms/names | Search RPMs |


<a name="documentation-for-models"></a>
## Documentation for Models

 - [api.GenericAttributeValidationResponse](./Models/api.GenericAttributeValidationResponse.md)
 - [api.Links](./Models/api.Links.md)
 - [api.RepositoryBulkCreateResponse](./Models/api.RepositoryBulkCreateResponse.md)
 - [api.RepositoryCollectionResponse](./Models/api.RepositoryCollectionResponse.md)
 - [api.RepositoryParameterResponse](./Models/api.RepositoryParameterResponse.md)
 - [api.RepositoryRequest](./Models/api.RepositoryRequest.md)
 - [api.RepositoryResponse](./Models/api.RepositoryResponse.md)
 - [api.RepositoryRpm](./Models/api.RepositoryRpm.md)
 - [api.RepositoryRpmCollectionResponse](./Models/api.RepositoryRpmCollectionResponse.md)
 - [api.RepositoryValidationRequest](./Models/api.RepositoryValidationRequest.md)
 - [api.RepositoryValidationResponse](./Models/api.RepositoryValidationResponse.md)
 - [api.ResponseMetadata](./Models/api.ResponseMetadata.md)
 - [api.SearchRpmRequest](./Models/api.SearchRpmRequest.md)
 - [api.SearchRpmResponse](./Models/api.SearchRpmResponse.md)
 - [api.UrlValidationResponse](./Models/api.UrlValidationResponse.md)
 - [config.DistributionArch](./Models/config.DistributionArch.md)
 - [config.DistributionVersion](./Models/config.DistributionVersion.md)


<a name="documentation-for-authorization"></a>
## Documentation for Authorization

<a name="RhIdentity"></a>
### RhIdentity

- **Type**: API key
- **API key parameter name**: x-rh-identity
- **Location**: HTTP header

