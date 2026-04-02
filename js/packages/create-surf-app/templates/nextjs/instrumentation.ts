export async function register() {
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    if (!process.env.SURF_API_KEY) {
      console.warn('\n⚠ SURF_API_KEY is not set. Add it to .env to enable Surf data API and database.\n')
    }

    const { syncSchema, watchSchema, startCron } = await import('./lib/boot')
    await syncSchema()
    if (process.env.NODE_ENV === 'development') {
      await watchSchema()
    }
    await startCron()
  }
}
