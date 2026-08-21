import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'

export type Theme = 'graphite' | 'midnight' | 'light'

export const THEMES: { id: Theme; label: string; hint: string }[] = [
  { id: 'graphite', label: 'Graphite', hint: 'Warm dark grey' },
  { id: 'midnight', label: 'Midnight', hint: 'Deep blue-black' },
  { id: 'light', label: 'Light', hint: 'Bright' },
]

const STORAGE_KEY = 'ccpm-theme'
const DEFAULT_THEME: Theme = 'graphite'

function isTheme(v: unknown): v is Theme {
  return v === 'graphite' || v === 'midnight' || v === 'light'
}

function readStoredTheme(): Theme {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (isTheme(v)) return v
  } catch {
    /* localStorage unavailable — fall through to default */
  }
  return DEFAULT_THEME
}

const ThemeContext = createContext<{
  theme: Theme
  setTheme: (t: Theme) => void
}>({ theme: DEFAULT_THEME, setTheme: () => {} })

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readStoredTheme)

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  function setTheme(next: Theme) {
    setThemeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* ignore persistence failure */
    }
  }

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  return useContext(ThemeContext)
}
