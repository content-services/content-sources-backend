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
 * @interface ErrorsHandlerError
 */
export interface ErrorsHandlerError {
    /**
     * An explanation specific to the problem
     * @type {string}
     * @memberof ErrorsHandlerError
     */
    detail?: string;
    /**
     * HTTP status code applicable to the error
     * @type {number}
     * @memberof ErrorsHandlerError
     */
    status?: number;
    /**
     * A summary of the problem
     * @type {string}
     * @memberof ErrorsHandlerError
     */
    title?: string;
}

/**
 * Check if a given object implements the ErrorsHandlerError interface.
 */
export function instanceOfErrorsHandlerError(value: object): value is ErrorsHandlerError {
    return true;
}

export function ErrorsHandlerErrorFromJSON(json: any): ErrorsHandlerError {
    return ErrorsHandlerErrorFromJSONTyped(json, false);
}

export function ErrorsHandlerErrorFromJSONTyped(json: any, ignoreDiscriminator: boolean): ErrorsHandlerError {
    if (json == null) {
        return json;
    }
    return {
        
        'detail': json['detail'] == null ? undefined : json['detail'],
        'status': json['status'] == null ? undefined : json['status'],
        'title': json['title'] == null ? undefined : json['title'],
    };
}

export function ErrorsHandlerErrorToJSON(json: any): ErrorsHandlerError {
    return ErrorsHandlerErrorToJSONTyped(json, false);
}

export function ErrorsHandlerErrorToJSONTyped(value?: ErrorsHandlerError | null, ignoreDiscriminator: boolean = false): any {
    if (value == null) {
        return value;
    }

    return {
        
        'detail': value['detail'],
        'status': value['status'],
        'title': value['title'],
    };
}

