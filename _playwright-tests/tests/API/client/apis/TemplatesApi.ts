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
  ApiSnapshotErrataCollectionResponse,
  ApiTemplateCollectionResponse,
  ApiTemplateRequest,
  ApiTemplateResponse,
  ApiTemplateUpdateRequest,
  ErrorsErrorResponse,
} from '../models/index';
import {
    ApiSnapshotErrataCollectionResponseFromJSON,
    ApiSnapshotErrataCollectionResponseToJSON,
    ApiTemplateCollectionResponseFromJSON,
    ApiTemplateCollectionResponseToJSON,
    ApiTemplateRequestFromJSON,
    ApiTemplateRequestToJSON,
    ApiTemplateResponseFromJSON,
    ApiTemplateResponseToJSON,
    ApiTemplateUpdateRequestFromJSON,
    ApiTemplateUpdateRequestToJSON,
    ErrorsErrorResponseFromJSON,
    ErrorsErrorResponseToJSON,
} from '../models/index';

export interface CreateTemplateRequest {
    apiTemplateRequest: ApiTemplateRequest;
}

export interface DeleteTemplateRequest {
    uuid: string;
}

export interface FullUpdateTemplateRequest {
    uuid: string;
    apiTemplateUpdateRequest: ApiTemplateUpdateRequest;
}

export interface GetTemplateRequest {
    uuid: string;
}

export interface GetTemplateRepoConfigurationFileRequest {
    templateUuid: string;
}

export interface ListTemplateErrataRequest {
    uuid: string;
    limit?: number;
    offset?: number;
    search?: string;
    type?: string;
    severity?: string;
    sortBy?: string;
}

export interface ListTemplatesRequest {
    offset?: number;
    limit?: number;
    version?: string;
    arch?: string;
    name?: string;
    repositoryUuids?: string;
    snapshotUuids?: string;
    sortBy?: string;
}

export interface PartialUpdateTemplateRequest {
    uuid: string;
    apiTemplateUpdateRequest: ApiTemplateUpdateRequest;
}

/**
 * 
 */
export class TemplatesApi extends runtime.BaseAPI {

    /**
     * This operation enables creating templates based on user preferences.
     * Create Template
     */
    async createTemplateRaw(requestParameters: CreateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiTemplateResponse>> {
        if (requestParameters['apiTemplateRequest'] == null) {
            throw new runtime.RequiredError(
                'apiTemplateRequest',
                'Required parameter "apiTemplateRequest" was null or undefined when calling createTemplate().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        headerParameters['Content-Type'] = 'application/json';

        const response = await this.request({
            path: `/templates/`,
            method: 'POST',
            headers: headerParameters,
            query: queryParameters,
            body: ApiTemplateRequestToJSON(requestParameters['apiTemplateRequest']),
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiTemplateResponseFromJSON(jsonValue));
    }

    /**
     * This operation enables creating templates based on user preferences.
     * Create Template
     */
    async createTemplate(requestParameters: CreateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiTemplateResponse> {
        const response = await this.createTemplateRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * This enables deleting a specific template.
     * Delete a template
     */
    async deleteTemplateRaw(requestParameters: DeleteTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<void>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling deleteTemplate().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/templates/{uuid}`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'DELETE',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.VoidApiResponse(response);
    }

    /**
     * This enables deleting a specific template.
     * Delete a template
     */
    async deleteTemplate(requestParameters: DeleteTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<void> {
        await this.deleteTemplateRaw(requestParameters, initOverrides);
    }

    /**
     * This operation enables updating all attributes of a template
     * Fully update all attributes of a Template
     */
    async fullUpdateTemplateRaw(requestParameters: FullUpdateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiTemplateResponse>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling fullUpdateTemplate().'
            );
        }

        if (requestParameters['apiTemplateUpdateRequest'] == null) {
            throw new runtime.RequiredError(
                'apiTemplateUpdateRequest',
                'Required parameter "apiTemplateUpdateRequest" was null or undefined when calling fullUpdateTemplate().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        headerParameters['Content-Type'] = 'application/json';

        const response = await this.request({
            path: `/templates/{uuid}`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'PUT',
            headers: headerParameters,
            query: queryParameters,
            body: ApiTemplateUpdateRequestToJSON(requestParameters['apiTemplateUpdateRequest']),
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiTemplateResponseFromJSON(jsonValue));
    }

    /**
     * This operation enables updating all attributes of a template
     * Fully update all attributes of a Template
     */
    async fullUpdateTemplate(requestParameters: FullUpdateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiTemplateResponse> {
        const response = await this.fullUpdateTemplateRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * Get template information.
     * Get Template
     */
    async getTemplateRaw(requestParameters: GetTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiTemplateResponse>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling getTemplate().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/templates/{uuid}`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiTemplateResponseFromJSON(jsonValue));
    }

    /**
     * Get template information.
     * Get Template
     */
    async getTemplate(requestParameters: GetTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiTemplateResponse> {
        const response = await this.getTemplateRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * Get configuration file for all repositories in a template
     */
    async getTemplateRepoConfigurationFileRaw(requestParameters: GetTemplateRepoConfigurationFileRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<string>> {
        if (requestParameters['templateUuid'] == null) {
            throw new runtime.RequiredError(
                'templateUuid',
                'Required parameter "templateUuid" was null or undefined when calling getTemplateRepoConfigurationFile().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/templates/{template_uuid}/config.repo`.replace(`{${"template_uuid"}}`, encodeURIComponent(String(requestParameters['templateUuid']))),
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        if (this.isJsonMime(response.headers.get('content-type'))) {
            return new runtime.JSONApiResponse<string>(response);
        } else {
            return new runtime.TextApiResponse(response) as any;
        }
    }

    /**
     * Get configuration file for all repositories in a template
     */
    async getTemplateRepoConfigurationFile(requestParameters: GetTemplateRepoConfigurationFileRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<string> {
        const response = await this.getTemplateRepoConfigurationFileRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * List errata in a content template.
     * List Template Errata
     */
    async listTemplateErrataRaw(requestParameters: ListTemplateErrataRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiSnapshotErrataCollectionResponse>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling listTemplateErrata().'
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

        if (requestParameters['type'] != null) {
            queryParameters['type'] = requestParameters['type'];
        }

        if (requestParameters['severity'] != null) {
            queryParameters['severity'] = requestParameters['severity'];
        }

        if (requestParameters['sortBy'] != null) {
            queryParameters['sort_by'] = requestParameters['sortBy'];
        }

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/templates/{uuid}/errata`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiSnapshotErrataCollectionResponseFromJSON(jsonValue));
    }

    /**
     * List errata in a content template.
     * List Template Errata
     */
    async listTemplateErrata(requestParameters: ListTemplateErrataRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiSnapshotErrataCollectionResponse> {
        const response = await this.listTemplateErrataRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * This operation enables users to retrieve a list of templates.
     * List Templates
     */
    async listTemplatesRaw(requestParameters: ListTemplatesRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiTemplateCollectionResponse>> {
        const queryParameters: any = {};

        if (requestParameters['offset'] != null) {
            queryParameters['offset'] = requestParameters['offset'];
        }

        if (requestParameters['limit'] != null) {
            queryParameters['limit'] = requestParameters['limit'];
        }

        if (requestParameters['version'] != null) {
            queryParameters['version'] = requestParameters['version'];
        }

        if (requestParameters['arch'] != null) {
            queryParameters['arch'] = requestParameters['arch'];
        }

        if (requestParameters['name'] != null) {
            queryParameters['name'] = requestParameters['name'];
        }

        if (requestParameters['repositoryUuids'] != null) {
            queryParameters['repository_uuids'] = requestParameters['repositoryUuids'];
        }

        if (requestParameters['snapshotUuids'] != null) {
            queryParameters['snapshot_uuids'] = requestParameters['snapshotUuids'];
        }

        if (requestParameters['sortBy'] != null) {
            queryParameters['sort_by'] = requestParameters['sortBy'];
        }

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/templates/`,
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiTemplateCollectionResponseFromJSON(jsonValue));
    }

    /**
     * This operation enables users to retrieve a list of templates.
     * List Templates
     */
    async listTemplates(requestParameters: ListTemplatesRequest = {}, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiTemplateCollectionResponse> {
        const response = await this.listTemplatesRaw(requestParameters, initOverrides);
        return await response.value();
    }

    /**
     * This operation enables updating some subset of attributes of a template
     * Update some attributes of a Template
     */
    async partialUpdateTemplateRaw(requestParameters: PartialUpdateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<ApiTemplateResponse>> {
        if (requestParameters['uuid'] == null) {
            throw new runtime.RequiredError(
                'uuid',
                'Required parameter "uuid" was null or undefined when calling partialUpdateTemplate().'
            );
        }

        if (requestParameters['apiTemplateUpdateRequest'] == null) {
            throw new runtime.RequiredError(
                'apiTemplateUpdateRequest',
                'Required parameter "apiTemplateUpdateRequest" was null or undefined when calling partialUpdateTemplate().'
            );
        }

        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        headerParameters['Content-Type'] = 'application/json';

        const response = await this.request({
            path: `/templates/{uuid}`.replace(`{${"uuid"}}`, encodeURIComponent(String(requestParameters['uuid']))),
            method: 'PATCH',
            headers: headerParameters,
            query: queryParameters,
            body: ApiTemplateUpdateRequestToJSON(requestParameters['apiTemplateUpdateRequest']),
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => ApiTemplateResponseFromJSON(jsonValue));
    }

    /**
     * This operation enables updating some subset of attributes of a template
     * Update some attributes of a Template
     */
    async partialUpdateTemplate(requestParameters: PartialUpdateTemplateRequest, initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<ApiTemplateResponse> {
        const response = await this.partialUpdateTemplateRaw(requestParameters, initOverrides);
        return await response.value();
    }

}
