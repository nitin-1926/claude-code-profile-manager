// Mirrors internal/profile.ValidateName so the GUI rejects bad names before
// shelling out (the CLI validates again — this is just fast feedback).
export function validateProfileName(name: string, taken: string[] = []): string | null {
  if (!name) return 'Name required'
  if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]*$/.test(name)) return 'Use letters, numbers, - or _ (must start alphanumeric)'
  if (name.length > 32) return 'Max 32 characters'
  if (taken.includes(name)) return `"${name}" already exists`
  return null
}
