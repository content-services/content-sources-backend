let counter = 1;

export const incrementNum = () => (((counter++ - 1) % 99) + 1).toString().padStart(2, '0');

// Generate new URL from sequence
export const generateNewUrl = () =>
  `https://content-services.github.io/fixtures/yum/centirepos/repo${incrementNum()}/`;
