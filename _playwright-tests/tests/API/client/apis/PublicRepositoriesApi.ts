/* tslint:disable */
/* eslint-disable */
/**
 * ContentSourcesBackend
 * The API for the repositories of the content sources that you can use to create and manage repositories between third-party applications and the [Red Hat Hybrid Cloud Console](https://console.redhat.com). With these repositories, you can build and deploy images using Image Builder for Cloud, on-Premise, and Edge. You can handle tasks, search for required RPMs, fetch a GPGKey from the URL, and list the features within applications. 
 *
 * The version of the OpenAPI document: v1.0.0
 * 
 *
 * NOTE: This class is auto generated by OpenAPI Generator (https://openapi-generator.tech).
 * https://openapi-generator.tech
 * Do not edit the class manually.
 */


import * as runtime from '../runtime';
import type {
  ApiPublicRepositoryCollectionResponse,
  ErrorsErrorResponse,
} from '../models/index';
import {
    ApiPublicRepositoryCollectionResponseFromJSON,
    ApiPublicRepositoryCollectionResponseToJSON,
    ErrorsErrorResponseFromJSON,
    ErrorsErrorResponseToJSON,
} from '../models/index';

export interface ListPublicRepositoriesRequest {
    offset?: number;
    limit?: number;
}

/**
 * 
 */
export class PublicRepositoriesApi extends runtime.BaseAPI {

    /**
     * Get public repositories. This enables listing a set of pre-created entries that represent a base set of RPMs needed for image building. These repositories are defined and made available to all user accounts, enabling them to perform RPM name searches using URLs as search criteria. These public repositories are not listed by the normal repositories API. It does not show up via the normal repositories API.
     * List Public Repositories
     */
    async listPublicRepositoriesRaw(requestParameters: ListPublicRepositoriesRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiPublicRepositoryCollectionResponse>> {
        const queryParameters: any = {};

        if (requestParameters['offset'] != null) {
            queryParameters['offset'] = requestParameters['offset'];
        }

        if (requestParameters['limit'] != null) {
            queryParameters['limit'] = requestParameters['limit'];
        }

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/public_repositories/`,
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiPublicRepositoryCollectionResponseFromJSON(jsonValue));
    }

    /**
     * Get public repositories. This enables listing a set of pre-created entries that represent a base set of RPMs needed for image building. These repositories are defined and made available to all user accounts, enabling them to perform RPM name searches using URLs as search criteria. These public repositories are not listed by the normal repositories API. It does not show up via the normal repositories API.
     * List Public Repositories
     */
    async listPublicRepositories(requestParameters: ListPublicRepositoriesRequest = {}, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiPublicRepositoryCollectionResponse> {
        const response = await this.listPublicRepositoriesRaw(requestParameters, initOverrides);
        return await response.value();
    }

}
