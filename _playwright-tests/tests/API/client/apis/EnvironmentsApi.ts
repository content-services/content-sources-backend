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
  ApiContentUnitSearchRequest,
  ApiRepositoryEnvironmentCollectionResponse,
  ApiSearchEnvironmentResponse,
  ApiSnapshotSearchRpmRequest,
  ErrorsErrorResponse,
} from '../models/index';
import {
    ApiContentUnitSearchRequestFromJSON,
    ApiContentUnitSearchRequestToJSON,
    ApiRepositoryEnvironmentCollectionResponseFromJSON,
    ApiRepositoryEnvironmentCollectionResponseToJSON,
    ApiSearchEnvironmentResponseFromJSON,
    ApiSearchEnvironmentResponseToJSON,
    ApiSnapshotSearchRpmRequestFromJSON,
    ApiSnapshotSearchRpmRequestToJSON,
    ErrorsErrorResponseFromJSON,
    ErrorsErrorResponseToJSON,
} from '../models/index';

export interface ListRepositoriesEnvironmentsRequest {
    uuid: string;
    limit?: number;
    offset?: number;
    search?: string;
    sortBy?: string;
}

export interface SearchEnvironmentsRequest {
    apiContentUnitSearchRequest: ApiContentUnitSearchRequest;
}

export interface SearchSnapshotEnvironmentsRequest {
    apiSnapshotSearchRpmRequest: ApiSnapshotSearchRpmRequest;
}

/**
 * 
 */
export class EnvironmentsApi extends runtime.BaseAPI {

    /**
     * List environments in a repository.
     * List Repositories Environments
     */
    async listRepositoriesEnvironmentsRaw(requestParameters: ListRepositoriesEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiRepositoryEnvironmentCollectionResponse>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling listRepositoriesEnvironments().'
            );
        }

        const queryParameters: any = {};

        if (requestParameters['limit'] != null) {
            queryParameters['limit'] = requestParameters['limit'];
        }

        if (requestParameters['offset'] != null) {
            queryParameters['offset'] = requestParameters['offset'];
        }

        if (requestParameters['search'] != null) {
            queryParameters['search'] = requestParameters['search'];
        }

        if (requestParameters['sortBy'] != null) {
            queryParameters['sort_by'] = requestParameters['sortBy'];
        }

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/repositories/{uuid}/environments`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiRepositoryEnvironmentCollectionResponseFromJSON(jsonValue));
    }

    /**
     * List environments in a repository.
     * List Repositories Environments
     */
    async listRepositoriesEnvironments(requestParameters: ListRepositoriesEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiRepositoryEnvironmentCollectionResponse> {
        const response = await this.listRepositoriesEnvironmentsRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * This enables users to search for environments in a given list of repositories.
     * Search environments
     */
    async searchEnvironmentsRaw(requestParameters: SearchEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<Array<ApiSearchEnvironmentResponse>>> {
        if (requestParameters['apiContentUnitSearchRequest'] == null) {
            throw new runtime.RequiredError(
                'apiContentUnitSearchRequest',
                'Required parameter "apiContentUnitSearchRequest" was null or undefined when calling searchEnvironments().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        headerParameters['Content-Type'] = 'application/json';

        const response = await this.request({
            path: `/environments/names`,
            method: 'POST',
            headers: headerParameters,
            query: queryParameters,
            body: ApiContentUnitSearchRequestToJSON(requestParameters['apiContentUnitSearchRequest']),
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => jsonValue.map(ApiSearchEnvironmentResponseFromJSON));
    }

    /**
     * This enables users to search for environments in a given list of repositories.
     * Search environments
     */
    async searchEnvironments(requestParameters: SearchEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<Array<ApiSearchEnvironmentResponse>> {
        const response = await this.searchEnvironmentsRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * This enables users to search for environments in a given list of snapshots.
     * Search environments within snapshots
     */
    async searchSnapshotEnvironmentsRaw(requestParameters: SearchSnapshotEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<Array<ApiSearchEnvironmentResponse>>> {
        if (requestParameters['apiSnapshotSearchRpmRequest'] == null) {
            throw new runtime.RequiredError(
                'apiSnapshotSearchRpmRequest',
                'Required parameter "apiSnapshotSearchRpmRequest" was null or undefined when calling searchSnapshotEnvironments().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        headerParameters['Content-Type'] = 'application/json';

        const response = await this.request({
            path: `/snapshots/environments/names`,
            method: 'POST',
            headers: headerParameters,
            query: queryParameters,
            body: ApiSnapshotSearchRpmRequestToJSON(requestParameters['apiSnapshotSearchRpmRequest']),
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => jsonValue.map(ApiSearchEnvironmentResponseFromJSON));
    }

    /**
     * This enables users to search for environments in a given list of snapshots.
     * Search environments within snapshots
     */
    async searchSnapshotEnvironments(requestParameters: SearchSnapshotEnvironmentsRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<Array<ApiSearchEnvironmentResponse>> {
        const response = await this.searchSnapshotEnvironmentsRaw(requestParameters, initOverrides);
        return await response.value();
    }

}
