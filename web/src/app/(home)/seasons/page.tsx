import { permanentRedirect } from 'next/navigation';

// /seasons was a separate page listing years and quarters. Those are now two
// filters on /browse, so the page has nothing left to be. The route stays as a
// redirect because it was live and linkable.
export default function SeasonsIndex(): never {
  permanentRedirect('/browse?kind=releases');
}
