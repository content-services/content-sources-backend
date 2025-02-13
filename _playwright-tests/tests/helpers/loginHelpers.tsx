import path from 'path';
import { exec } from 'child_process';

export const setAuthorizationHeader = async (userName: string, org: number) => {
  process.env.IDENTITY_HEADER = decodeURI(
    await new Promise((resolve, reject) => {
      exec(
        `"${path.join(__dirname, '../../../scripts/header.sh')}" ${org} ${userName}`,
        (error, stdout, stderr) => {
          if (error) {
            reject(`Error: ${error.message}`);
          } else if (stderr) {
            reject(`Stderr: ${stderr}`);
          } else {
            resolve(stdout);
          }
        },
      );
    }),
  )
    .replace('x-rh-identity: ', '')
    .trim();
};

export const throwIfMissingEnvVariables = () => {
  const ManditoryEnvVariables = ['BASE_URL'];

  const missing: string[] = [];
  ManditoryEnvVariables.forEach((envVar) => {
    if (!process.env[envVar]) {
      missing.push(envVar);
    }
  });

  if (missing.length > 0) {
    throw new Error('Missing env variables:' + missing.join(','));
  }
};
