export type ComboValue = string | number;

export type ComboPlacement =
  | 'top-right'
  | 'top-left'
  | 'top-center'
  | 'bottom-right'
  | 'bottom-left'
  | 'bottom-center';

export type ComboAnchor =
  | 'top-center'
  | 'top-left'
  | 'top-right'
  | 'bottom-center'
  | 'bottom-left'
  | 'bottom-right'
  | 'center';

export interface ComboOption {
  value: ComboValue;
  label: string;
  sublabel?: string;
  disabled?: boolean;
  selected?: boolean;
}

export interface ComboSection {
  id?: string;
  title?: string;
  options: ComboOption[];
}
