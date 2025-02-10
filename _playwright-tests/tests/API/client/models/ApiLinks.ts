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
 * @interface ApiLinks
 */
export interface ApiLinks {
    /**
     * Path to first page of results
     * @type {string}
     * @memberof ApiLinks
     */
    first?: string;
    /**
     * Path to last page of results
     * @type {string}
     * @memberof ApiLinks
     */
    last?: string;
    /**
     * Path to next page of results
     * @type {string}
     * @memberof ApiLinks
     */
    next?: string;
    /**
     * Path to previous page of results
     * @type {string}
     * @memberof ApiLinks
     */
    prev?: string;
}

/**
 * Check if a given object implements the ApiLinks interface.
 */
export function instanceOfApiLinks(value: object): value is ApiLinks {
    return true;
}

export function ApiLinksFromJSON(json: any): ApiLinks {
    return ApiLinksFromJSONTyped(json, false);
}

export function ApiLinksFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiLinks {
    if (json == null) {
        return json;
    }
    return {
        
        'first': json['first'] == null ? undefined : json['first'],
        'last': json['last'] == null ? undefined : json['last'],
        'next': json['next'] == null ? undefined : json['next'],
        'prev': json['prev'] == null ? undefined : json['prev'],
    };
}

export function ApiLinksToJSON(json: any): ApiLinks {
    return ApiLinksToJSONTyped(json, false);
}

export function ApiLinksToJSONTyped(value?: ApiLinks | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'first': value['first'],
        'last': value['last'],
        'next': value['next'],
        'prev': value['prev'],
    };
}

