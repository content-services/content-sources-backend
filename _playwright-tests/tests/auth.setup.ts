import { expect, test as setup } from '@playwright/test';
import { setAuthorizationHeader, throwIfMissingEnvVariables } from './helpers/loginHelpers';
import { describe } from 'node:test';

const DefaultOrg = 99999;
const DefaultUser = 'BananaMan';

describe('Setup', async () => {
  setup('Ensure needed ENV variables exist', async () => {
    expect(() => throwIfMissingEnvVariables()).not.toThrow();
  });

  setup('Authenticate', async () => {
    if (!process.env['IDENTITY_HEADER']) {
      await setAuthorizationHeader(DefaultUser, DefaultOrg);
    }
  });
});
