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

import { mapValues } from '../runtime';
import type { ApiResponseMetadata } from './ApiResponseMetadata';
import {
    ApiResponseMetadataFromJSON,
    ApiResponseMetadataFromJSONTyped,
    ApiResponseMetadataToJSON,
    ApiResponseMetadataToJSONTyped,
} from './ApiResponseMetadata';
import type { ApiLinks } from './ApiLinks';
import {
    ApiLinksFromJSON,
    ApiLinksFromJSONTyped,
    ApiLinksToJSON,
    ApiLinksToJSONTyped,
} from './ApiLinks';
import type { ApiSnapshotResponse } from './ApiSnapshotResponse';
import {
    ApiSnapshotResponseFromJSON,
    ApiSnapshotResponseFromJSONTyped,
    ApiSnapshotResponseToJSON,
    ApiSnapshotResponseToJSONTyped,
} from './ApiSnapshotResponse';

/**
 * 
 * @export
 * @interface ApiSnapshotCollectionResponse
 */
export interface ApiSnapshotCollectionResponse {
    /**
     * Requested Data
     * @type {Array<ApiSnapshotResponse>}
     * @memberof ApiSnapshotCollectionResponse
     */
    data?: Array<ApiSnapshotResponse>;
    /**
     * 
     * @type {ApiLinks}
     * @memberof ApiSnapshotCollectionResponse
     */
    links?: ApiLinks;
    /**
     * 
     * @type {ApiResponseMetadata}
     * @memberof ApiSnapshotCollectionResponse
     */
    meta?: ApiResponseMetadata;
}

/**
 * Check if a given object implements the ApiSnapshotCollectionResponse interface.
 */
export function instanceOfApiSnapshotCollectionResponse(value: object): value is ApiSnapshotCollectionResponse {
    return true;
}

export function ApiSnapshotCollectionResponseFromJSON(json: any): ApiSnapshotCollectionResponse {
    return ApiSnapshotCollectionResponseFromJSONTyped(json, false);
}

export function ApiSnapshotCollectionResponseFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiSnapshotCollectionResponse {
    if (json == null) {
        return json;
    }
    return {
        
        'data': json['data'] == null ? undefined : ((json['data'] as Array<any>).map(ApiSnapshotResponseFromJSON)),
        'links': json['links'] == null ? undefined : ApiLinksFromJSON(json['links']),
        'meta': json['meta'] == null ? undefined : ApiResponseMetadataFromJSON(json['meta']),
    };
}

export function ApiSnapshotCollectionResponseToJSON(json: any): ApiSnapshotCollectionResponse {
    return ApiSnapshotCollectionResponseToJSONTyped(json, false);
}

export function ApiSnapshotCollectionResponseToJSONTyped(value?: ApiSnapshotCollectionResponse | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'data': value['data'] == null ? undefined : ((value['data'] as Array<any>).map(ApiSnapshotResponseToJSON)),
        'links': ApiLinksToJSON(value['links']),
        'meta': ApiResponseMetadataToJSON(value['meta']),
    };
}

