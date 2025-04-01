export const randomName = () => (Math.random() + 1).toString(36).substring(2, 6);

export const rank = () => Math.floor(Math.random() * 10 + 1).toString();
export const randomNum = () =>
  Math.floor(Math.random() * 10 + 1)
    .toString()
    .padStart(2, '0');

export const randomUrl = () =>
  `https://stephenw.fedorapeople.org/multirepos/${rank()}/repo${randomNum()}/`;

export const SmallRedHatRepoURL =
  'https://cdn.redhat.com/content/dist/rhel9/9/aarch64/codeready-builder/os/';
