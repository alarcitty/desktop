import poster1 from '../assets/releases/v48_451.png';
import poster2 from '../assets/releases/v48_453.png';
import poster3 from '../assets/releases/v48_455.png';
import poster4 from '../assets/releases/v48_457.png';
import kaguya from '../assets/recommendations/v49_566.png';
import dandadan from '../assets/recommendations/v49_569.png';
import dressUp from '../assets/recommendations/v49_572.png';
import alya from '../assets/library/alya.png';

export interface AnimeEntry
{
  id: string;
  title: string;
  year: number;
  poster: string;
  posterPosition?: string;
}

// Temporary local catalog for the search popup (will be replaced by the real backend).
export const catalog: AnimeEntry[] = [
  { id: 'mushoku', title: 'Mushoku Tensei: Jobless Reincarnation', year: 2021, poster: poster1 },
  { id: 'black-clover', title: 'Black Clover', year: 2017, poster: poster2 },
  { id: 'frieren', title: "Frieren: Beyond Journey's End", year: 2023, poster: poster3 },
  { id: 'tsukimichi', title: 'Tsukimichi: Moonlit Fantasy', year: 2021, poster: poster4, posterPosition: 'left center' },
  { id: 'alya', title: 'Alya Sometimes Hides Her Feelings in Russian', year: 2024, poster: alya },
  { id: 'kaguya', title: 'Kaguya-sama: Love Is War', year: 2019, poster: kaguya },
  { id: 'dandadan', title: 'Dan Da Dan', year: 2024, poster: dandadan },
  { id: 'dress-up', title: 'My Dress-Up Darling', year: 2022, poster: dressUp }
];
