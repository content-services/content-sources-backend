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
/**
 * 
 * @export
 * @interface ApiResponseMetadata
 */
export interface ApiResponseMetadata {
    /**
     * Total count of results
     * @type {number}
     * @memberof ApiResponseMetadata
     */
    count?: number;
    /**
     * Limit of results used for the request
     * @type {number}
     * @memberof ApiResponseMetadata
     */
    limit?: number;
    /**
     * Offset into results used for the request
     * @type {number}
     * @memberof ApiResponseMetadata
     */
    offset?: number;
}

/**
 * Check if a given object implements the ApiResponseMetadata interface.
 */
export function instanceOfApiResponseMetadata(value: object): value is ApiResponseMetadata {
    return true;
}

export function ApiResponseMetadataFromJSON(json: any): ApiResponseMetadata {
    return ApiResponseMetadataFromJSONTyped(json, false);
}

export function ApiResponseMetadataFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiResponseMetadata {
    if (json == null) {
        return json;
    }
    return {
        
        'count': json['count'] == null ? undefined : json['count'],
        'limit': json['limit'] == null ? undefined : json['limit'],
        'offset': json['offset'] == null ? undefined : json['offset'],
    };
}

export function ApiResponseMetadataToJSON(json: any): ApiResponseMetadata {
    return ApiResponseMetadataToJSONTyped(json, false);
}

export function ApiResponseMetadataToJSONTyped(value?: ApiResponseMetadata | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'count': value['count'],
        'limit': value['limit'],
        'offset': value['offset'],
    };
}

