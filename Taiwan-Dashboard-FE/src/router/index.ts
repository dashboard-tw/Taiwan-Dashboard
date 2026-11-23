import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import EmbedView from '../views/EmbedView.vue'
import { EXTRA_DASHBOARDS, TOPICS } from '../lib/topics'
import { isChunkError, reloadForNewBuild } from '../lib/staleBuild'

const topic = `:topic(${[...TOPICS.map((t) => t.slug), ...EXTRA_DASHBOARDS].join('|')})`
const county = ':county(\\d{5})'
const typhoonId = ':id(\\d{4}-\\d{1,3})'

export const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    if (to.hash) return { el: to.hash, top: to.name === 'developers' ? 70 : 0 }
    if (to.params.topic && to.params.topic === from.params.topic) return false
    return { top: 0 }
  },
  routes: [
    { path: '/', name: 'home', component: DashboardView, props: { slug: 'home' } },
    {
      path: `/typhoon/${typhoonId}`,
      name: 'typhoon-event',
      component: DashboardView,
      props: (r) => ({ slug: 'typhoon-event', typhoonId: r.params.id }),
    },
    {
      path: `/typhoon/${typhoonId}/map`,
      name: 'typhoon-event-map',
      component: DashboardView,
      props: (r) => ({ slug: 'typhoon-event', typhoonId: r.params.id, map: true }),
    },
    {
      path: `/typhoon/${typhoonId}/${county}`,
      name: 'typhoon-event-county',
      component: DashboardView,
      props: (r) => ({ slug: 'typhoon-event', typhoonId: r.params.id, county: r.params.county }),
    },
    {
      path: `/typhoon/${typhoonId}/${county}/map`,
      name: 'typhoon-event-county-map',
      component: DashboardView,
      props: (r) => ({
        slug: 'typhoon-event',
        typhoonId: r.params.id,
        county: r.params.county,
        map: true,
      }),
    },
    {
      path: '/earthquake/report/:id(\\d{4,20})',
      name: 'earthquake-report',
      component: () => import('../views/EarthquakeReportView.vue'),
      props: true,
    },
    {
      path: '/typhoons',
      name: 'typhoons',
      component: () => import('../views/TyphoonListView.vue'),
    },
    {
      path: `/${topic}`,
      name: 'topic',
      component: DashboardView,
      props: (r) => ({ slug: r.params.topic }),
    },
    {
      path: `/${topic}/map`,
      name: 'topic-map',
      component: DashboardView,
      props: (r) => ({ slug: r.params.topic, map: true }),
    },
    {
      path: `/${topic}/${county}`,
      name: 'topic-county',
      component: DashboardView,
      props: (r) => ({ slug: r.params.topic, county: r.params.county }),
    },
    {
      path: `/${topic}/${county}/map`,
      name: 'topic-county-map',
      component: DashboardView,
      props: (r) => ({ slug: r.params.topic, county: r.params.county, map: true }),
    },
    {
      path: `/county/:code(\\d{5})`,
      name: 'county',
      component: () => import('../views/CountyView.vue'),
      props: true,
    },
    {
      path: '/components/:slug',
      name: 'component',
      component: () => import('../views/ComponentView.vue'),
      props: true,
    },
    {
      path: '/embed/:slug',
      name: 'embed',
      component: EmbedView,
      props: true,
      meta: { bare: true },
    },
    {
      path: '/components',
      name: 'components',
      component: () => import('../views/ComponentLibraryView.vue'),
    },
    { path: '/login', name: 'login', component: { render: () => null } },
    {
      path: '/me',
      name: 'account',
      component: () => import('../views/me/AccountView.vue'),
      meta: { auth: true },
    },
    {
      path: '/me/devices',
      name: 'devices',
      component: () => import('../views/me/DevicesView.vue'),
      meta: { auth: true },
    },
    {
      path: '/me/showcase',
      name: 'my-apps',
      component: () => import('../views/me/MyAppsView.vue'),
      meta: { auth: true },
    },
    {
      path: '/me/api-key',
      name: 'api-key',
      component: () => import('../views/me/ApiKeyView.vue'),
      meta: { auth: true },
    },
    {
      path: '/oauth/authorize',
      name: 'oauth-authorize',
      component: () => import('../views/OAuthAuthorizeView.vue'),
    },
    { path: '/about', name: 'about', component: () => import('../views/AboutView.vue') },
    {
      path: '/showcase/:category(app|web|innovation)',
      name: 'showcase',
      component: () => import('../views/AppsView.vue'),
      props: true,
    },
    { path: '/showcase', redirect: '/showcase/app' },
    { path: '/apps', redirect: '/showcase/app' },
    { path: '/me/apps', redirect: '/me/showcase' },
    {
      path: '/developers/:section?/:item?',
      name: 'developers',
      component: () => import('../views/DevelopersView.vue'),
      meta: { bare: true },
    },
    { path: '/docs/api', redirect: '/developers' },
    { path: '/sitemap', name: 'sitemap', component: () => import('../views/SitemapView.vue') },
    { path: '/privacy', name: 'privacy', component: () => import('../views/PrivacyView.vue') },
    { path: '/terms', name: 'terms', component: () => import('../views/TermsView.vue') },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/NotFoundView.vue'),
    },
  ],
})

async function openLogin(redirect: string, error: string | undefined, hasCurrentPage: boolean) {
  const { useDialogs } = await import('../stores/dialog')
  useDialogs().openLogin(redirect, error)
  return hasCurrentPage ? false : { path: '/' }
}

router.beforeEach(async (to, from) => {
  const hasCurrentPage = from.matched.length > 0
  if (to.name === 'login') {
    const r = to.query.redirect
    const e = to.query.error
    return openLogin(
      typeof r === 'string' ? r : '/',
      typeof e === 'string' ? e : undefined,
      hasCurrentPage,
    )
  }
  if (to.path === '/admin' || to.path.startsWith('/admin/')) {
    const { useAuth } = await import('../stores/auth')
    const auth = useAuth()
    await auth.load(true)
    if (auth.isAdmin && !router.hasRoute('admin')) {
      const { adminRoute } = await import('./adminRoutes')
      router.addRoute(adminRoute)
      return to.fullPath
    }
  }
  if (!to.meta.admin && !to.meta.auth) return true
  const { useAuth } = await import('../stores/auth')
  const auth = useAuth()
  if (to.meta.admin) {
    await auth.load(true)
    if (auth.isAdmin) return true
    return {
      name: 'not-found',
      params: { pathMatch: to.path.slice(1).split('/') },
      query: to.query,
      hash: to.hash,
      replace: true,
    }
  }
  await auth.load()
  if (auth.me) return true
  return openLogin(to.fullPath, undefined, hasCurrentPage)
})

router.onError((err, to) => {
  if (isChunkError(err)) reloadForNewBuild(to.fullPath)
})
