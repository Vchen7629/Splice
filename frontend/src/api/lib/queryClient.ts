// Creates a react query client for managing fetching lifecycle
// (caching, refetching, retries), more robust
import { QueryClient } from "@tanstack/react-query"

export const queryClient = new QueryClient()