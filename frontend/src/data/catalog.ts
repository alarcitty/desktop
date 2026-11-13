export interface AnimeEntry
{
  id: string;
  shikiId: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  year: number;
  poster: string;
  posterPosition?: string;
}

export const catalog: AnimeEntry[] = [
  {
    id: 'apothecary',
    shikiId: 54492,
    searchQueries: ['Kusuriya no Hitorigoto', 'Монолог фармацевта'],
    title: 'The Apothecary Diaries',
    year: 2023,
    poster: 'https://shikimori.io/uploads/poster/animes/54492/81c8c3df5a5e6b1b6d3c739e23650f22.jpeg'
  },
  {
    id: 'mushoku',
    shikiId: 39535,
    searchQueries: ['Mushoku Tensei', 'Реинкарнация безработного'],
    title: 'Mushoku Tensei: Jobless Reincarnation',
    year: 2021,
    poster: 'https://shikimori.io/uploads/poster/animes/39535/f66d91c3fdc65449f4bb3568e4d36fec.jpeg'
  },
  {
    id: 'black-clover',
    shikiId: 34572,
    searchQueries: ['Black Clover', 'Чёрный клевер'],
    title: 'Black Clover',
    year: 2017,
    poster: 'https://shikimori.io/uploads/poster/animes/34572/d4c5bf9cac831c03fc5ebed2676b7b10.jpeg'
  },
  {
    id: 'frieren',
    shikiId: 52991,
    releaseAlias: 'sousou-no-frieren',
    searchQueries: ['Sousou no Frieren', 'Провожающая в последний путь Фрирен'],
    title: "Frieren: Beyond Journey's End",
    year: 2023,
    poster: 'https://shikimori.io/uploads/poster/animes/52991/dc841cc9fce2aa1e9907a4b61c5d1d92.jpeg'
  },
  {
    id: 'tsukimichi',
    shikiId: 43523,
    releaseAlias: 'tsuki-ga-michibiku-isekai-douchuu',
    searchQueries: ['Tsuki ga Michibiku Isekai Douchuu', 'Лунное путешествие'],
    title: 'Tsukimichi: Moonlit Fantasy',
    year: 2021,
    poster: 'https://shikimori.io/uploads/poster/animes/43523/9f56a98052f05ad81a43452ecacf657d.jpeg'
  },
  {
    id: 'alya',
    shikiId: 54744,
    searchQueries: ['Tokidoki Bosotto', 'Аля иногда кокетничает'],
    title: 'Alya Sometimes Hides Her Feelings in Russian',
    year: 2024,
    poster: 'https://shikimori.io/uploads/poster/animes/54744/de6fc036f101d94a4f738be3f0415d2c.jpeg'
  },
  {
    id: 'kaguya',
    shikiId: 52198,
    releaseAlias: 'kaguya-sama-wa-kokurasetai-tensai-tachi-no-renai-zunousen',
    searchQueries: ['Kaguya-sama', 'Госпожа Кагуя'],
    title: 'Kaguya-sama: Love Is War',
    year: 2019,
    poster: 'https://shikimori.io/uploads/poster/animes/52198/85bd1b1c1919611ca9fbf46d2d404558.jpeg'
  },
  {
    id: 'dandadan',
    shikiId: 57334,
    releaseAlias: 'dandadan',
    searchQueries: ['Dandadan', 'Дандадан'],
    title: 'Dan Da Dan',
    year: 2024,
    poster: 'https://shikimori.io/uploads/poster/animes/57334/17926e9e162dcf37b3e4533561ff712d.jpeg'
  },
  {
    id: 'dress-up',
    shikiId: 48736,
    releaseAlias: 'sono-bisque-doll-wa-koi-wo-suru',
    searchQueries: ['Sono Bisque Doll', 'Эта фарфоровая кукла'],
    title: 'My Dress-Up Darling',
    year: 2022,
    poster: 'https://shikimori.io/uploads/poster/animes/48736/536021070760d16f823b485bc5b021a6.jpeg'
  },
  {
    id: 'solo-leveling',
    shikiId: 52299,
    searchQueries: ['Ore dake Level Up na Ken', 'Поднятие уровня в одиночку'],
    title: 'Solo Leveling',
    year: 2024,
    poster: 'https://shikimori.io/uploads/poster/animes/52299/9a1ee4e32c72ab99b76e84bbe49f7e87.jpeg'
  }
];
