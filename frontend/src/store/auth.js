import { defineStore } from 'pinia'
import api from '@/api/client'

// Mirrors GET /api/v1/me's response shape exactly (see
// backend/internal/handlers/auth_handler.go meHandler): {authenticated,
// user:{id,signet_id,name,email,role,status,on_vacation,roc_status}}.
export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null,
    registrationPath: '',
    authenticated: false,
    bootstrapped: false,
  }),
  getters: {
    role: (state) => state.user?.role || null,
    isCompany: (state) => state.user?.role === 'company',
    isAdmin: (state) => state.user?.role === 'admin',
    isAgent: (state) => state.user?.role === 'agent',
    isUser: (state) => state.user?.role === 'user',
    landingRoute: (state) => state.registrationPath || (state.user ? `/${state.user.role}/dashboard` : '/login'),
    dashboardRoute: (state) => (state.user ? `/${state.user.role}/dashboard` : '/login'),
  },
  actions: {
    async bootstrap() {
      try {
        const { data } = await api.get('/me')
        this.authenticated = !!data.authenticated
        this.user = data.user || null
        this.registrationPath = data.registration_path || ''
      } catch {
        this.authenticated = false
        this.user = null
        this.registrationPath = ''
      } finally {
        this.bootstrapped = true
      }
    },
    async login(email, password) {
      const { data } = await api.post('/login', { email, password })
      if (data.status === 'success') {
        await this.bootstrap()
      }
      return data
    },
    async logout() {
      try {
        await api.post('/logout')
      } finally {
        this.forceLogout()
      }
    },
    forceLogout() {
      this.authenticated = false
      this.user = null
      this.registrationPath = ''
      if (window.location.pathname !== '/login') {
        window.location.assign('/login')
      }
    },
  },
})
