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
 * @interface ApiSearchEnvironmentResponse
 */
export interface ApiSearchEnvironmentResponse {
    /**
     * Description of the environment found
     * @type {string}
     * @memberof ApiSearchEnvironmentResponse
     */
    description?: string;
    /**
     * Environment found
     * @type {string}
     * @memberof ApiSearchEnvironmentResponse
     */
    environmentName?: string;
    /**
     * ID of the environment found
     * @type {string}
     * @memberof ApiSearchEnvironmentResponse
     */
    id?: string;
}

/**
 * Check if a given object implements the ApiSearchEnvironmentResponse interface.
 */
export function instanceOfApiSearchEnvironmentResponse(value: object): value is ApiSearchEnvironmentResponse {
    return true;
}

export function ApiSearchEnvironmentResponseFromJSON(json: any): ApiSearchEnvironmentResponse {
    return ApiSearchEnvironmentResponseFromJSONTyped(json, false);
}

export function ApiSearchEnvironmentResponseFromJSONTyped(json: any, ignoreDiscriminator: boolean): ApiSearchEnvironmentResponse {
    if (json == null) {
        return json;
    }
    return {
        
        'description': json['description'] == null ? undefined : json['description'],
        'environmentName': json['environment_name'] == null ? undefined : json['environment_name'],
        'id': json['id'] == null ? undefined : json['id'],
    };
}

export function ApiSearchEnvironmentResponseToJSON(json: any): ApiSearchEnvironmentResponse {
    return ApiSearchEnvironmentResponseToJSONTyped(json, false);
}

export function ApiSearchEnvironmentResponseToJSONTyped(value?: ApiSearchEnvironmentResponse | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'description': value['description'],
        'environment_name': value['environmentName'],
        'id': value['id'],
    };
}

