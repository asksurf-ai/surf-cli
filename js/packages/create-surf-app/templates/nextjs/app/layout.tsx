import type { Metadata } from "next"
import { ThemeProvider } from "next-themes"
import { Providers } from "./providers"
import "./globals.css"

export const metadata: Metadata = {
  title: "Surf App",
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false}>
          <Providers>{children}</Providers>
        </ThemeProvider>
      </body>
    </html>
  )
}
