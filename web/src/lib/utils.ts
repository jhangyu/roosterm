import { clsx, type ClassValue } from "clsx"
import { extendTailwindMerge } from "tailwind-merge"

// Names mirror the @theme scales in styles/globals.css; without them twMerge reads the mockup's
// text-<size> utilities as colors (and drops the real text color) and shadow-N / rounded-X as unknown.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: ["caption", "body", "subtitle2", "subtitle1", "title3"],
      shadow: ["2", "4", "8", "16", "28", "64"],
      radius: ["none", "xs", "sm", "md", "lg", "xl", "full"],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
