import { useMutation } from '@tanstack/react-query'
import { api } from '@/api/client'
import type { LoginResponse } from '@/api/types'

interface LoginInput {
  email: string
  password: string
  totp_code?: string
}

export function useLogin() {
  return useMutation({
    mutationFn: async (input: LoginInput): Promise<LoginResponse> => {
      const { data } = await api.post<LoginResponse>('/admin/login', input)
      return data
    },
  })
}
