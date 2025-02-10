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
 * @interface ApiRepositoryUpdateRequest
 */
export interface ApiRepositoryUpdateRequest {
    /**
     * Architecture to restrict client usage to
     * @type {string}
     * @memberof ApiRepositoryUpdateRequest
     */
    distributionArch?: string;
    /**
     * Versions to restrict client usage to
     * @type {Array<string>}
     * @memberof ApiRepositoryUpdateRequest
     */
    distributionVersions?: Array<string>;
    /**
     * GPG key for repository
     * @type {string}
     * @memberof ApiRepositoryUpdateRequest
     */
    gpgKey?: string;
    /**
     * Verify packages
     * @type {boolean}
     * @memberof ApiRepositoryUpdateRequest
     */
    metadataVerification?: boolean;
    /**
     * Disable modularity filtering on this repository
     * @type {boolean}
     * @memberof ApiRepositoryUpdateRequest
     */
    moduleHotfixes?: boolean;
    /**
     * Name of the remote yum repository
     * @type {string}
     * @memberof ApiRepositoryUpdateRequest
     */
    name?: string;
    /**
     * Enable snapshotting and hosting of this repository
     * @type {boolean}
     * @memberof ApiRepositoryUpdateRequest
     */
    snapshot?: boolean;
    /**
     * URL of the remote yum repository
     * @type {string}
     * @memberof ApiRepositoryUpdateRequest
     */
    url?: string;
}

/**
 * Check if a given object implements the ApiRepositoryUpdateRequest interface.
 */
export function instanceOfApiRepositoryUpdateRequest(value: object): value is ApiRepositoryUpdateRequest {
    return true;
}

export function ApiRepositoryUpdateRequestFromJSON(json: any): ApiRepositoryUpdateRequest {
    return ApiRepositoryUpdateRequestFromJSONTyped(json, false);
}

export function ApiRepositoryUpdateRequestFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiRepositoryUpdateRequest {
    if (json == null) {
        return json;
    }
    return {
        
        'distributionArch': json['distribution_arch'] == null ? undefined : json['distribution_arch'],
        'distributionVersions': json['distribution_versions'] == null ? undefined : json['distribution_versions'],
        'gpgKey': json['gpg_key'] == null ? undefined : json['gpg_key'],
        'metadataVerification': json['metadata_verification'] == null ? undefined : json['metadata_verification'],
        'moduleHotfixes': json['module_hotfixes'] == null ? undefined : json['module_hotfixes'],
        'name': json['name'] == null ? undefined : json['name'],
        'snapshot': json['snapshot'] == null ? undefined : json['snapshot'],
        'url': json['url'] == null ? undefined : json['url'],
    };
}

export function ApiRepositoryUpdateRequestToJSON(json: any): ApiRepositoryUpdateRequest {
    return ApiRepositoryUpdateRequestToJSONTyped(json, false);
}

export function ApiRepositoryUpdateRequestToJSONTyped(value?: ApiRepositoryUpdateRequest | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'distribution_arch': value['distributionArch'],
        'distribution_versions': value['distributionVersions'],
        'gpg_key': value['gpgKey'],
        'metadata_verification': value['metadataVerification'],
        'module_hotfixes': value['moduleHotfixes'],
        'name': value['name'],
        'snapshot': value['snapshot'],
        'url': value['url'],
    };
}

