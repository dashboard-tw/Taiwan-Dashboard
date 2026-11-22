import type { RouteRecordRaw } from 'vue-router'

export const adminRoute: RouteRecordRaw = {
  path: '/admin',
  component: () => import('../views/admin/AdminLayout.vue'),
  meta: { admin: true },
  children: [
    { path: '', name: 'admin', component: () => import('../views/admin/AdminHomeView.vue') },
    {
      path: 'pipeline',
      name: 'admin-pipeline',
      component: () => import('../views/admin/AdminPipelineView.vue'),
    },
    {
      path: 'components',
      name: 'admin-components',
      component: () => import('../views/admin/AdminComponentsView.vue'),
    },
    {
      path: 'components/new',
      name: 'admin-component-new',
      component: () => import('../views/admin/AdminComponentEditView.vue'),
    },
    {
      path: 'components/:slug',
      name: 'admin-component',
      component: () => import('../views/admin/AdminComponentEditView.vue'),
      props: true,
    },
    {
      path: 'users',
      name: 'admin-users',
      component: () => import('../views/admin/AdminUsersView.vue'),
    },
    {
      path: 'apps',
      name: 'admin-apps',
      component: () => import('../views/admin/AdminAppsView.vue'),
    },
    {
      path: 'api-usage',
      name: 'admin-api-usage',
      component: () => import('../views/admin/AdminApiUsageView.vue'),
    },
    {
      path: 'api-survey',
      name: 'admin-api-survey',
      component: () => import('../views/admin/AdminSurveyView.vue'),
    },
    {
      path: 'reservoirs',
      name: 'admin-reservoirs',
      component: () => import('../views/admin/AdminReservoirsView.vue'),
    },
    {
      path: 'power-units',
      name: 'admin-power-units',
      component: () => import('../views/admin/AdminPowerUnitsView.vue'),
    },
  ],
}
