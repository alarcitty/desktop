export interface AnimeEntry
{
  id: string;
  shikiId: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  year: number;
  score: number;
  episodesTotal?: number;
  typeLabel?: string;
  genres?: string[];
  poster: string;
  posterPosition?: string;
}

export const catalog: AnimeEntry[] = [
  {
    id: 'apothecary',
    shikiId: 54492,
    releaseAlias: 'kusuriya-no-hitorigoto',
    searchQueries: ['Kusuriya no Hitorigoto', 'The Apothecary Diaries'],
    title: 'The Apothecary Diaries',
    year: 2023,
    score: 8.9,
    episodesTotal: 24,
    typeLabel: 'TV',
    genres: ['Drama', 'Mystery'],
    poster: 'https://shikimori.io/uploads/poster/animes/54492/283fbef180e72bacfadc5a3d64ca4c2e.jpeg'
  },
  {
    id: 'mushoku',
    shikiId: 39535,
    releaseAlias: 'mushoku-tensei-isekai-ittara-honki-dasu',
    searchQueries: ['Mushoku Tensei', 'Mushoku Tensei: Jobless Reincarnation'],
    title: 'Mushoku Tensei: Jobless Reincarnation',
    year: 2021,
    score: 8.4,
    episodesTotal: 23,
    typeLabel: 'TV',
    genres: ['Fantasy', 'Drama'],
    poster: 'https://shikimori.io/uploads/poster/animes/39535/f66d91c3fdc65449f4bb3568e4d36fec.jpeg'
  },
  {
    id: 'black-clover',
    shikiId: 34572,
    releaseAlias: 'black-clover',
    searchQueries: ['Black Clover'],
    title: 'Black Clover',
    year: 2017,
    score: 8.1,
    episodesTotal: 170,
    typeLabel: 'TV',
    genres: ['Action', 'Fantasy'],
    poster: 'https://shikimori.io/uploads/poster/animes/34572/d4c5bf9cac831c03fc5ebed2676b7b10.jpeg'
  },
  {
    id: 'frieren',
    shikiId: 52991,
    releaseAlias: 'sousou-no-frieren',
    searchQueries: ['Sousou no Frieren', "Frieren: Beyond Journey's End"],
    title: "Frieren: Beyond Journey's End",
    year: 2023,
    score: 9.3,
    episodesTotal: 28,
    typeLabel: 'TV',
    genres: ['Fantasy', 'Adventure'],
    poster: 'https://shikimori.io/uploads/poster/animes/52991/dc841cc9fce2aa1e9907a4b61c5d1d92.jpeg'
  },
  {
    id: 'tsukimichi',
    shikiId: 43523,
    releaseAlias: 'tsuki-ga-michibiku-isekai-douchuu',
    searchQueries: ['Tsuki ga Michibiku Isekai Douchuu', 'Tsukimichi: Moonlit Fantasy'],
    title: 'Tsukimichi: Moonlit Fantasy',
    year: 2021,
    score: 7.7,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Fantasy', 'Action'],
    poster: 'https://shikimori.io/uploads/poster/animes/43523/9f56a98052f05ad81a43452ecacf657d.jpeg'
  },
  {
    id: 'alya',
    shikiId: 54744,
    releaseAlias: 'tokidoki-bosotto-russia-go-de-dereru-tonari-no-alya-san',
    searchQueries: ['Tokidoki Bosotto', 'Alya Sometimes Hides Her Feelings in Russian'],
    title: 'Alya Sometimes Hides Her Feelings in Russian',
    year: 2024,
    score: 7.6,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Romance', 'Comedy'],
    poster: 'https://shikimori.io/uploads/poster/animes/54744/de6fc036f101d94a4f738be3f0415d2c.jpeg'
  },
  {
    id: 'kaguya',
    shikiId: 52198,
    releaseAlias: 'kaguya-sama-wa-kokurasetai-tensai-tachi-no-renai-zunousen',
    searchQueries: ['Kaguya-sama', 'Kaguya-sama: Love Is War'],
    title: 'Kaguya-sama: Love Is War',
    year: 2019,
    score: 8.9,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Romance', 'Comedy'],
    poster: 'https://shikimori.io/uploads/poster/animes/52198/85bd1b1c1919611ca9fbf46d2d404558.jpeg'
  },
  {
    id: 'dandadan',
    shikiId: 57334,
    releaseAlias: 'dandadan',
    searchQueries: ['Dandadan', 'Dan Da Dan'],
    title: 'Dan Da Dan',
    year: 2024,
    score: 8.6,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Action', 'Supernatural'],
    poster: 'https://shikimori.io/uploads/poster/animes/57334/17926e9e162dcf37b3e4533561ff712d.jpeg'
  },
  {
    id: 'dress-up',
    shikiId: 48736,
    releaseAlias: 'sono-bisque-doll-wa-koi-wo-suru',
    searchQueries: ['Sono Bisque Doll', 'My Dress-Up Darling'],
    title: 'My Dress-Up Darling',
    year: 2022,
    score: 8.2,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Romance', 'Slice of Life'],
    poster: 'https://shikimori.io/uploads/poster/animes/48736/536021070760d16f823b485bc5b021a6.jpeg'
  },
  {
    id: 'solo-leveling',
    shikiId: 52299,
    releaseAlias: 'ore-dake-level-up-na-ken',
    searchQueries: ['Ore dake Level Up na Ken', 'Solo Leveling'],
    title: 'Solo Leveling',
    year: 2024,
    score: 8.3,
    episodesTotal: 12,
    typeLabel: 'TV',
    genres: ['Action', 'Fantasy'],
    poster: 'https://shikimori.io/uploads/poster/animes/52299/9a1ee4e32c72ab99b76e84bbe49f7e87.jpeg'
  }
];
