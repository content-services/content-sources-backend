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
 * @interface ConfigDistributionVersion
 */
export interface ConfigDistributionVersion {
    /**
     * Static label of the version
     * @type {string}
     * @memberof ConfigDistributionVersion
     */
    label?: string;
    /**
     * Human-readable form of the version
     * @type {string}
     * @memberof ConfigDistributionVersion
     */
    name?: string;
}

/**
 * Check if a given object implements the ConfigDistributionVersion interface.
 */
export function instanceOfConfigDistributionVersion(value: object): value is ConfigDistributionVersion {
    return true;
}

export function ConfigDistributionVersionFromJSON(json: any): ConfigDistributionVersion {
    return ConfigDistributionVersionFromJSONTyped(json, false);
}

export function ConfigDistributionVersionFromJSONTyped(json: any, ignoreDiscriminator: boolean): ConfigDistributionVersion {
    if (json == null) {
        return json;
    }
    return {
        
        'label': json['label'] == null ? undefined : json['label'],
        'name': json['name'] == null ? undefined : json['name'],
    };
}

export function ConfigDistributionVersionToJSON(json: any): ConfigDistributionVersion {
    return ConfigDistributionVersionToJSONTyped(json, false);
}

export function ConfigDistributionVersionToJSONTyped(value?: ConfigDistributionVersion | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'label': value['label'],
        'name': value['name'],
    };
}

