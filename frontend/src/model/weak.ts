/** The list with the item removed if it is there, or added if it is not. */
export const toggled = (list: string[], item: string): string[] => (list.includes(item) ? list.filter((x) => x !== item) : [...list, item])
