export const randomName = () => (Math.random() + 1).toString(36).substring(2, 6);

export const randomNum = () =>
  Math.floor(Math.random() * 100 + 1)
    .toString()
    .padStart(2, '0');

export const randomUrl = () =>
  `https://content-services.github.io/fixtures/yum/centirepos/repo${randomNum()}/`;

export const SmallRedHatRepoURL =
  'https://cdn.redhat.com/content/dist/rhel9/9/aarch64/codeready-builder/os/';
