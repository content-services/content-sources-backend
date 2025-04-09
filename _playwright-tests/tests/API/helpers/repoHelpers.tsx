export const randomName = () => (Math.random() + 1).toString(36).substring(2, 6);

export const randomNum = () =>
  Math.floor(Math.random() * 100 + 1)
    .toString()
    .padStart(2, '0');

export const randomUrl = () =>
  `https://stephenw.fedorapeople.org/centirepos/repo${randomNum()}/`;
