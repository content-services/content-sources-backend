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
 * @interface ApiRepositoryRpm
 */
export interface ApiRepositoryRpm {
    /**
     * The architecture of the rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    arch?: string;
    /**
     * The checksum of the rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    checksum?: string;
    /**
     * The epoch of the rpm
     * @type {number}
     * @memberof ApiRepositoryRpm
     */
    epoch?: number;
    /**
     * The rpm package name
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    name?: string;
    /**
     * The release of the rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    release?: string;
    /**
     * The summary of the rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    summary?: string;
    /**
     * Identifier of the rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    uuid?: string;
    /**
     * The version of the  rpm
     * @type {string}
     * @memberof ApiRepositoryRpm
     */
    version?: string;
}

/**
 * Check if a given object implements the ApiRepositoryRpm interface.
 */
export function instanceOfApiRepositoryRpm(value: object): value is ApiRepositoryRpm {
    return true;
}

export function ApiRepositoryRpmFromJSON(json: any): ApiRepositoryRpm {
    return ApiRepositoryRpmFromJSONTyped(json, false);
}

export function ApiRepositoryRpmFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiRepositoryRpm {
    if (json == null) {
        return json;
    }
    return {
        
        'arch': json['arch'] == null ? undefined : json['arch'],
        'checksum': json['checksum'] == null ? undefined : json['checksum'],
        'epoch': json['epoch'] == null ? undefined : json['epoch'],
        'name': json['name'] == null ? undefined : json['name'],
        'release': json['release'] == null ? undefined : json['release'],
        'summary': json['summary'] == null ? undefined : json['summary'],
        'uuid': json['uuid'] == null ? undefined : json['uuid'],
        'version': json['version'] == null ? undefined : json['version'],
    };
}

export function ApiRepositoryRpmToJSON(json: any): ApiRepositoryRpm {
    return ApiRepositoryRpmToJSONTyped(json, false);
}

export function ApiRepositoryRpmToJSONTyped(value?: ApiRepositoryRpm | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'arch': value['arch'],
        'checksum': value['checksum'],
        'epoch': value['epoch'],
        'name': value['name'],
        'release': value['release'],
        'summary': value['summary'],
        'uuid': value['uuid'],
        'version': value['version'],
    };
}

