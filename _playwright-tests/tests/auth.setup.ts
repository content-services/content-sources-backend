import { expect, test as setup} from '@playwright/test';
import { setAuthorizationHeader, throwIfMissingEnvVariables } from './helpers/loginHelpers';
import { describe } from 'node:test';

describe("Setup", async () => {
    setup('Ensure needed ENV variables exist', async () => {
        expect(() => throwIfMissingEnvVariables()).not.toThrow()
    })

    setup('Authenticate', async ({ page }) => {
        await setAuthorizationHeader("BananaMan", 99999)
    })
})
