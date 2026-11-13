export type ComboValue = string | number;

export type ComboPlacement = 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';

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
