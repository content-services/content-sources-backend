import { expect } from '@playwright/test';
import { test } from "./base_client";
import { FeaturesApi } from "./client";


test('Content > GetFeatures API', async ({ client }) => {
    const resp  = await new FeaturesApi(client).listFeatures()        
    expect(resp["snapshots"]["enabled"]).toBe(true)
});