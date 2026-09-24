<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContentIframeSrc"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- 品牌首页直接由 Home 渲染，复用应用路由、登录状态和后台模型配置。 -->
  <div v-else-if="!classicHomeEnabled" class="brand-home" :class="{ 'is-dark': isDark }" data-testid="brand-home">
    <header class="header">
      <div class="nav">
        <router-link class="brand" :to="{ hash: '#home', query: route.query }" :aria-label="brandSiteName + brandT('home')"><span class="logo"><img v-if="siteLogo" :src="siteLogo" alt="" /><template v-else>智</template></span><span>{{ brandSiteName }}</span></router-link>
        <nav class="desktop-nav" aria-label="主要导航">
          <router-link class="nav-link" :class="{ active: brandView === 'home' }" :to="{ hash: '#home', query: route.query }">{{ brandT('home') }}</router-link>
          <router-link class="nav-link" :class="{ active: brandView === 'pricing' }" :to="{ hash: '#pricing', query: route.query }">{{ brandT('pricing') }}</router-link>
        </nav>
        <div class="actions">
          <button class="icon-button language-button" id="languageButton" type="button" :disabled="brandLocaleSwitching" aria-label="切换语言 / Switch language" @click="toggleBrandLocale">{{ brandLocale === 'zh' ? '文A' : 'A文' }}</button>
          <button class="icon-button" id="themeButton" type="button" :aria-label="brandT(isDark ? 'toLight' : 'toDark')" :title="brandT(isDark ? 'toLight' : 'toDark')" @click="toggleTheme"><Icon :name="isDark ? 'sun' : 'moon'" size="md" /></button>
          <router-link class="auth-button login-button shine guest-only" to="/login" v-if="!isAuthenticated">{{ brandT('login') }}</router-link>
          <router-link class="auth-button register-button guest-only" to="/register" v-if="!isAuthenticated">{{ brandT('register') }}</router-link>
          <router-link class="auth-button console-button shine auth-only" :to="dashboardPath" v-if="isAuthenticated">
            <span>{{ brandT('console') }}</span>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M7 17 17 7M7 7h10v10"/></svg>
          </router-link>
          <button class="menu-button" id="menuButton" type="button" :aria-label="brandT(brandMenuOpen ? 'closeMenu' : 'openMenu')" :aria-expanded="brandMenuOpen" aria-controls="mobileNav" @click="brandMenuOpen = !brandMenuOpen"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path :d="brandMenuOpen ? 'M6 18 18 6M6 6l12 12' : 'M4 7h16M4 12h16M4 17h16'" /></svg></button>
        </div>
      </div>
      <nav class="mobile-nav" id="mobileNav" aria-label="移动端导航" :hidden="!brandMenuOpen" @click="brandMenuOpen = false" @keydown.esc="brandMenuOpen = false">
        <router-link :to="{ hash: '#home', query: route.query }">{{ brandT('home') }}</router-link>
        <router-link :to="{ hash: '#pricing', query: route.query }">{{ brandT('pricing') }}</router-link>
        <router-link class="guest-only" to="/login" v-if="!isAuthenticated">{{ brandT('login') }}</router-link>
        <router-link class="guest-only" to="/register" v-if="!isAuthenticated">{{ brandT('register') }}</router-link>
        <router-link class="auth-only" :to="dashboardPath" v-if="isAuthenticated">{{ brandT('console') }}</router-link>
      </nav>
    </header>

    <main>
      <section class="view" id="homeView" :hidden="brandView !== 'home'">
        <div class="hero">
          <div class="hero-glow" aria-hidden="true"></div>
          <h1>{{ brandSiteName }}</h1>
          <p class="brand-subtitle">{{ brandT('subtitle') }}</p>
          <p class="tagline">{{ brandT('tagline') }}</p>
          <router-link class="primary-cta shine" id="primaryCta" :to="isAuthenticated ? dashboardPath : '/register'"><span>{{ brandT(isAuthenticated ? 'console' : 'getKey') }}</span><svg v-if="isAuthenticated" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M7 17 17 7M7 7h10v10" /></svg></router-link>
          <div class="stats" aria-label="平台数据">
            <div class="stat"><strong id="modelCount">{{ modelLoadState === 'loaded' ? homeModels.length : '—' }}</strong><span>{{ brandT('modelsAvailable') }}</span></div>
            <div class="stat"><strong>99.9%</strong><span>{{ brandT('availability') }}</span></div>
            <div class="stat"><strong>200ms</strong><span>{{ brandT('latency') }}</span></div>
          </div>
        </div>
        <section class="features" id="support" aria-label="平台能力">
          <article class="feature-card"><div class="feature-icon"><svg width="25" height="25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 20V9l8-5 8 5v11M2 20h20M8 20v-6h8v6"/></svg></div><h2>{{ brandT('pool') }}</h2><p>{{ brandT('poolDesc') }}</p></article>
          <article class="feature-card"><div class="feature-icon"><svg width="25" height="25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="m13 2-9 12h7l-1 8 9-12h-7l1-8Z"/></svg></div><h2>{{ brandT('speed') }}</h2><p>{{ brandT('speedDesc') }}</p></article>
          <article class="feature-card"><div class="feature-icon"><svg width="25" height="25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3v8Z"/><path d="m9 12 2 2 4-4"/></svg></div><h2>{{ brandT('reliable') }}</h2><p>{{ brandT('reliableDesc') }}</p></article>
          <article class="feature-card"><div class="feature-icon"><svg width="25" height="25" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 3v18h18M7 15l4-4 3 3 5-7"/></svg></div><h2>{{ brandT('billing') }}</h2><p>{{ brandT('billingDesc') }}</p></article>
        </section>
        <section class="families" aria-label="支持的模型家族"><span>GPT</span><span>Claude</span></section>
      </section>

      <section class="view pricing-view" id="pricingView" :hidden="brandView !== 'pricing'">
        <div v-if="modelLoadState === 'loaded' && homeModels.length" class="filters" id="filters" aria-label="模型类型筛选">
          <button v-for="filter in modelFilters" :key="filter.type" class="filter-button" :class="{ active: modelType === filter.type }" type="button" :data-filter-type="filter.type" :aria-pressed="modelType === filter.type" @click="modelType = filter.type">
            <span>{{ brandT(filter.label) }}</span><span class="filter-count">{{ filter.count }}</span>
          </button>
        </div>
        <section class="model-grid" id="modelGrid" aria-live="polite" :aria-busy="modelLoadState === 'loading'">
          <div v-if="modelLoadState === 'loading'" class="model-status"><span>{{ brandT('loadingModels') }}</span></div>
          <div v-else-if="modelLoadState === 'error'" class="model-status" role="alert">
            <strong>{{ brandT('loadFailed') }}</strong><span>{{ brandT('loadFailedDesc') }}</span>
            <button class="retry-button" type="button" data-retry-models @click="loadHomeModels">{{ brandT('retry') }}</button>
          </div>
          <div v-else-if="filteredHomeModels.length === 0" class="model-status"><span>{{ brandT('emptyModels') }}</span></div>
          <template v-else>
            <article v-for="(model, index) in filteredHomeModels" :key="index" class="model-card" :class="{ 'image-card': model.type === 'image' }">
              <div class="model-head">
                <h2 class="model-name">{{ model.name }}</h2>
                <span class="vendor" :class="getBrandVendor(model.vendor) ? 'vendor-' + model.vendor.trim().toLowerCase() : ''">
                  <span v-if="getBrandVendor(model.vendor)" class="vendor-icon" aria-hidden="true"><svg :viewBox="getBrandVendor(model.vendor)?.viewBox" fill="currentColor"><path :d="getBrandVendor(model.vendor)?.path" /></svg></span>
                  <span class="vendor-label">{{ getBrandVendor(model.vendor)?.label || model.vendor }}</span>
                </span>
              </div>
              <template v-if="model.type === 'text'">
                <div class="cache-badge">{{ brandT('tokenUnit') }}</div>
                <div class="price-grid">
                  <div class="price"><span>{{ brandT('input') }}</span><strong>{{ formatHomePrice(model.input) }}<small>/M</small></strong></div>
                  <div class="price"><span>{{ brandT('output') }}</span><strong>{{ formatHomePrice(model.output) }}<small>/M</small></strong></div>
                </div>
                <hr />
                <div class="cache-row"><span>{{ brandT('cachedInput') }}</span><strong>{{ formatHomePrice(model.cachedInput) }}/M</strong></div>
                <div class="cache-row"><span>{{ brandT('flexInput') }}</span><strong>{{ formatHomePrice(model.flexInput) }}/M</strong></div>
              </template>
              <template v-else>
                <div class="cache-badge">{{ brandT('imageGeneration') }}</div>
                <div class="image-price-grid">
                  <div v-for="size in imageResolutions" :key="size" class="image-price"><span>{{ size }}</span><strong>{{ formatHomePrice(model.resolutionPrices[size]) }}</strong><small>{{ brandT('perImage') }}</small></div>
                </div>
                <p class="image-note">{{ brandT('resolutionPricing') }}</p>
              </template>
            </article>
          </template>
        </section>
      </section>
    </main>
    <footer class="footer">&copy; <span id="year">{{ currentYear }}</span> {{ brandSiteName }}. <span>{{ brandT('rights') }}</span></footer>
  </div>

  <!-- 经典首页可通过 ?view=classic 访问。 -->
  <div
    v-else
    class="relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"
      ></div>
      <div
        class="absolute bottom-1/4 right-1/4 h-64 w-64 rounded-full bg-primary-400/10 blur-3xl"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <div class="flex items-center">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
        </div>

        <!-- Nav Actions -->
        <div class="flex items-center gap-3">
          <!-- Language Switcher -->
          <LocaleSwitcher />

          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Model Plaza Link -->
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
            <svg
              class="h-3 w-3 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
              />
            </svg>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 py-16">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section - Left/Right Layout -->
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <!-- Left: Text Content -->
          <div class="flex-1 text-center lg:text-left">
            <h1
              class="mb-4 text-4xl font-bold text-gray-900 dark:text-white md:text-5xl lg:text-6xl"
            >
              {{ siteName }}
            </h1>
            <p class="mb-8 text-lg text-gray-600 dark:text-dark-300 md:text-xl">
              {{ siteSubtitle }}
            </p>

            <!-- CTA Button -->
            <div>
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
            </div>
          </div>

          <!-- Right: Terminal Animation -->
          <div class="flex flex-1 justify-center lg:justify-end">
            <div class="terminal-container">
              <div class="terminal-window">
                <!-- Window header -->
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <!-- Terminal content -->
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">/v1/messages</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># Routing to upstream...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Feature Tags - Centered -->
        <div class="mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6">
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="swap" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.subscriptionToApi')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="shield" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.stickySession')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="chart" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.realtimeBilling')
            }}</span>
          </div>
        </div>

        <!-- Features Grid -->
        <div class="mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-blue-500 to-blue-600 shadow-lg shadow-blue-500/30 transition-transform group-hover:scale-110"
            >
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.unifiedGateway') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.unifiedGatewayDesc') }}
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-purple-500 to-purple-600 shadow-lg shadow-purple-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </div>
        </div>

        <!-- Supported Providers -->
        <div class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('home.providers.title') }}
          </h2>
          <p class="text-sm text-gray-600 dark:text-dark-400">
            {{ t('home.providers.description') }}
          </p>
        </div>

        <div class="mb-16 flex flex-wrap items-center justify-center gap-4">
          <!-- Claude - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-400 to-orange-500"
            >
              <span class="text-xs font-bold text-white">C</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.claude') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- GPT - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-green-500 to-green-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">GPT</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Gemini - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-blue-500 to-blue-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.gemini') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Antigravity - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-rose-500 to-pink-600"
            >
              <span class="text-xs font-bold text-white">A</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.antigravity') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- More - Coming Soon -->
          <div
            class="flex items-center gap-2 rounded-xl border border-gray-200/50 bg-white/40 px-5 py-3 opacity-60 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/40"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-gray-500 to-gray-600"
            >
              <span class="text-xs font-bold text-white">+</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.more') }}</span>
            <span
              class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
        </div>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

import { setLocale } from '@/i18n'
import type { HomeModel } from '@/api/admin/homeModels'
import { brandMessages, formatHomePrice, getBrandVendor, isValidHomeModel, type BrandMessageKey } from './home/brandHome'

const { t, locale } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()
const route = useRoute()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
// 兼容已保存的旧首页地址，避免升级后再次进入静态页面 iframe。
const legacyBrandHome = computed(() => {
  try {
    const url = new URL(homeContent.value.trim(), window.location.origin)
    return url.origin === window.location.origin && url.pathname === '/i2.html' ? url : null
  } catch {
    return null
  }
})
const hasHomeContent = computed(() => homeContent.value.trim().length > 0 && !legacyBrandHome.value)
const compactHomeEnabled = computed(() => !legacyBrandHome.value && appStore.cachedPublicSettings?.compact_home_enabled === true)
const classicHomeEnabled = computed(() => !legacyBrandHome.value && route.query.view === 'classic')
const homeContentIframeSrc = computed(() => sanitizeUrl(homeContent.value, { allowRelative: true }))
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  return homeContentIframeSrc.value.length > 0
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// GitHub URL
const githubUrl = 'https://github.com/NieTen/sub2api'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

// 品牌首页保留原稿布局，动态数据通过 Vue 文本插值输出，避免模型配置注入 HTML。
const brandSiteName = computed(() => siteName.value === 'Sub2API' ? '众智AI' : siteName.value)
const brandLocale = computed(() => locale?.value === 'en' ? 'en' : 'zh')
const brandLocaleSwitching = ref(false)
const brandMenuOpen = ref(false)
const brandView = computed(() => (route.hash || legacyBrandHome.value?.hash) === '#pricing' ? 'pricing' : 'home')
const brandHomeActive = computed(() => !hasHomeContent.value && !compactHomeEnabled.value && !classicHomeEnabled.value)
const homeModels = ref<HomeModel[]>([])
const modelLoadState = ref<'loading' | 'loaded' | 'error'>('loading')
const modelType = ref<'all' | 'text' | 'image'>('all')
const imageResolutions = ['1K', '2K', '4K'] as const
const filteredHomeModels = computed(() => modelType.value === 'all' ? homeModels.value : homeModels.value.filter(model => model.type === modelType.value))
const modelFilters = computed(() => [
  { type: 'all' as const, label: 'all' as const, count: homeModels.value.length },
  { type: 'text' as const, label: 'textModels' as const, count: homeModels.value.filter(model => model.type === 'text').length },
  { type: 'image' as const, label: 'imageModels' as const, count: homeModels.value.filter(model => model.type === 'image').length },
])
let modelRequest: AbortController | undefined

function brandT(key: BrandMessageKey) {
  return brandMessages[brandLocale.value][key]
}

async function toggleBrandLocale() {
  if (brandLocaleSwitching.value) return
  brandLocaleSwitching.value = true
  try {
    await setLocale(brandLocale.value === 'zh' ? 'en' : 'zh')
  } finally {
    brandLocaleSwitching.value = false
  }
}

async function loadHomeModels() {
  modelRequest?.abort()
  const controller = new AbortController()
  modelRequest = controller
  modelLoadState.value = 'loading'
  const timeout = window.setTimeout(() => controller.abort(), 15000)
  try {
    const response = await fetch('/api/v1/settings/home-models', { cache: 'no-store', signal: controller.signal })
    if (!response.ok) throw new Error('模型接口请求失败')
    const payload = await response.json()
    if (payload.code !== 0 || !Array.isArray(payload.data) || !payload.data.every(isValidHomeModel)) throw new Error('模型数据格式无效')
    if (modelRequest !== controller) return
    homeModels.value = payload.data
    modelLoadState.value = 'loaded'
    if (modelType.value !== 'all' && !homeModels.value.some(model => model.type === modelType.value)) modelType.value = 'all'
  } catch {
    if (modelRequest !== controller) return
    homeModels.value = []
    modelLoadState.value = 'error'
  } finally {
    window.clearTimeout(timeout)
  }
}

watch(brandHomeActive, active => {
  if (active) void loadHomeModels()
  else {
    modelRequest?.abort()
    modelRequest = undefined
  }
}, { immediate: true })
watch(() => route.hash, () => { brandMenuOpen.value = false })
onBeforeUnmount(() => {
  modelRequest?.abort()
  modelRequest = undefined
})

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
/* 原始首页样式限制在品牌根节点内，不影响紧凑首页和经典首页。 */

    .brand-home {
      color-scheme: light;
      --page: #f9fafb;
      --card: rgba(255,255,255,.82);
      --text: #242b3a;
      --body: #6b7280;
      --muted: #8a90a1;
      --purple: #5b21b6;
      --purple-2: #a78bfa;
      --purple-light: #ede9fe;
      --line: #e5e7eb;
      --shadow: rgba(15,23,42,.05);
      --container: 1280px;
    }
    .brand-home.is-dark {
      color-scheme: dark;
      --page: #07111f;
      --card: rgba(25,35,53,.84);
      --text: #f3f4f6;
      --body: #aeb7c7;
      --muted: #8792a6;
      --purple: #a78bfa;
      --purple-2: #c4b5fd;
      --purple-light: rgba(124,58,237,.18);
      --line: #334155;
      --shadow: rgba(0,0,0,.24);
    }
    .brand-home * { box-sizing: border-box; }
    .brand-home { scroll-behavior: smooth; }
    .brand-home {
      margin: 0;
      min-width: 320px;
      overflow-x: hidden;
      color: var(--text);
      background: var(--page);
      font-family: system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"PingFang SC","Microsoft YaHei",sans-serif;
      line-height: normal;
      transition: color .24s ease,background-color .24s ease;
    }
    .brand-home button,.brand-home a { font: inherit; }
    .brand-home button { cursor: pointer; }
    .brand-home a { color: inherit;text-decoration: none; }
    .brand-home svg { display: block; }
    .brand-home :focus-visible { outline: 3px solid rgba(124,58,237,.42);outline-offset: 3px; }
    .brand-home [hidden] { display: none !important; }

    .brand-home {
      position: relative;
      min-height: 100vh;
      overflow: hidden;
      isolation: isolate;
      background:
        radial-gradient(circle at 50% 48%,rgba(139,92,246,.11),transparent 480px),
        linear-gradient(#fff,#fbfcff);
      transition: background .24s ease;
    }
    .brand-home::before {
      content: "";
      position: fixed;
      z-index: -2;
      inset: 0;
      pointer-events: none;
      background-image: radial-gradient(rgba(17,24,39,.12) 1px,transparent 1px);
      background-size: 24px 24px;
      opacity: .34;
      mask-image: linear-gradient(to bottom,#000,transparent 76%);
    }
    .brand-home::after {
      content: "";
      position: fixed;
      z-index: -1;
      top: 108px;
      left: 50%;
      width: min(76vw,920px);
      height: 560px;
      pointer-events: none;
      border-radius: 50%;
      background:
        radial-gradient(circle at 24% 58%,rgba(59,130,246,.11),transparent 52%),
        radial-gradient(circle at 72% 38%,rgba(139,92,246,.17),transparent 57%);
      filter: blur(24px);
      transform: translateX(-50%);
      animation: background-drift 10s ease-in-out infinite alternate;
    }
    .brand-home.is-dark {
      background:
        radial-gradient(circle at 50% 44%,rgba(124,58,237,.17),transparent 520px),
        linear-gradient(#081321,#07111f);
    }
    .brand-home.is-dark::before {
      background-image: radial-gradient(rgba(148,163,184,.18) 1px,transparent 1px);
      opacity: .25;
    }
    .brand-home.is-dark::after {
      background:
        radial-gradient(circle at 24% 58%,rgba(14,165,233,.14),transparent 50%),
        radial-gradient(circle at 72% 38%,rgba(139,92,246,.24),transparent 58%);
      opacity: .82;
    }
    @keyframes background-drift {
      from { transform: translateX(-50%) translate3d(-18px,-8px,0) scale(.98); }
      to { transform: translateX(-50%) translate3d(24px,14px,0) scale(1.04); }
    }

    .brand-home .header { position: relative;z-index: 20;height: 88px;padding: 20px 48px; }
    .brand-home .nav { height: 48px;display: grid;grid-template-columns: 1fr auto 1fr;align-items: center; }
    .brand-home .brand { justify-self: start;display: inline-flex;align-items: center;gap: 10px;font-size: 19.5px;font-weight: 850; }
    .brand-home .logo {
      width: 36px;height: 36px;display: grid;place-items: center;border-radius: 50%;
      color: #fff;background: linear-gradient(135deg,#7c3aed,#db2777);
      box-shadow: 0 8px 22px rgba(91,33,182,.24);font-size: 15px;font-weight: 900;
    }
    .brand-home .desktop-nav { display: flex;align-items: center;gap: 3px; }
    .brand-home .nav-link,.brand-home .icon-button,.brand-home .auth-button,.brand-home .menu-button,.brand-home .filter-button,.brand-home .primary-cta {
      border: 1px solid transparent;
      transition: color .2s ease,background .2s ease,border-color .2s ease,box-shadow .2s ease,transform .2s ease;
    }
    .brand-home .nav-link {
      min-height: 40px;display: inline-flex;align-items: center;border-radius: 999px;
      padding: 0 14px;color: var(--body);font-size: 15.2px;font-weight: 750;
    }
    .brand-home .nav-link:hover,.brand-home .nav-link.active { color: var(--purple);background: rgba(124,58,237,.07);border-color: rgba(124,58,237,.3); }
    .brand-home .actions { justify-self: end;display: flex;align-items: center;gap: 4px; }
    .brand-home .icon-button,.brand-home .menu-button {
      width: 40px;height: 38px;display: grid;place-items: center;border-radius: 999px;
      color: var(--body);background: transparent;
    }
    .brand-home .icon-button:hover,.brand-home .menu-button:hover { color: var(--purple);border-color: rgba(124,58,237,.38);background: rgba(255,255,255,.72); }
    .brand-home.is-dark .icon-button:hover,.brand-home.is-dark .menu-button:hover { background: rgba(255,255,255,.08); }
    .brand-home .language-button { width: 44px;font-size: 13px;font-weight: 850; }
    .brand-home .auth-button {
      position: relative;overflow: hidden;min-width: 64px;height: 38px;display: inline-flex;
      align-items: center;justify-content: center;gap: 6px;border-radius: 999px;padding: 0 15px;
      font-size: 14.4px;font-weight: 850;
    }
    .brand-home .auth-button:hover { transform: scale(1.06); }
    .brand-home .login-button { color: var(--purple);border-color: rgba(91,33,182,.34);background: rgba(255,255,255,.16); }
    .brand-home .register-button,.brand-home .console-button {
      color: #fff;background: linear-gradient(#5b21b6,#a78bfa);
      background-origin: border-box;background-repeat: no-repeat;
      box-shadow: 0 10px 20px rgba(91,33,182,.22);
    }
    .brand-home .register-button:hover,.brand-home .console-button:hover { border-color: rgba(255,255,255,.78);box-shadow: 0 14px 30px rgba(91,33,182,.36); }
    .brand-home .shine::after {
      content: "";position: absolute;inset: -160%;pointer-events: none;opacity: 0;
      background: linear-gradient(135deg,transparent 43.5%,rgba(167,139,250,.08) 47%,rgba(255,255,255,.92) 50%,rgba(196,181,253,.58) 53%,transparent 56.5%);
      transform: translate3d(-38%,-38%,0);will-change: transform,opacity;
    }
    .brand-home .shine:hover::after { animation: button-shine .84s cubic-bezier(.22,.61,.36,1) both; }
    @keyframes button-shine {
      0% { transform: translate3d(-38%,-38%,0);opacity: 0; }
      14% { opacity: 1; }
      86% { opacity: 1; }
      100% { transform: translate3d(38%,38%,0);opacity: 0; }
    }
    .brand-home .menu-button { display: none; }
    .brand-home .mobile-nav {
      position: absolute;top: 70px;right: 16px;left: 16px;display: grid;gap: 3px;padding: 10px;
      border: 1px solid var(--line);border-radius: 14px;background: var(--card);
      box-shadow: 0 18px 50px rgba(40,25,80,.13);backdrop-filter: blur(18px);
    }
    .brand-home .mobile-nav a { min-height: 42px;display: flex;align-items: center;border-radius: 10px;padding: 0 12px;color: var(--body); }
    .brand-home .mobile-nav a:hover { color: var(--purple);background: var(--purple-light); }

    .brand-home .view { width: min(var(--container),calc(100% - 32px));margin: auto; }
    .brand-home .hero {
      position: relative;overflow: hidden;min-height: 497px;padding: 4px 32px 45px;
      display: flex;flex-direction: column;align-items: center;text-align: center;
    }
    .brand-home .hero-glow {
      position: absolute;z-index: -1;top: 94px;left: 50%;width: 420px;height: 330px;
      pointer-events: none;background:
        radial-gradient(circle,rgba(139,92,246,.22),transparent 58%),
        radial-gradient(circle at 45% 65%,rgba(59,130,246,.1),transparent 62%);
      filter: blur(18px);transform: translateX(-50%);animation: hero-breathe 6s ease-in-out infinite;
    }
    @keyframes hero-breathe {
      0%,100% { opacity: .74;transform: translateX(-50%) scale(.96); }
      50% { opacity: 1;transform: translateX(-50%) scale(1.06); }
    }
    .brand-home h1 {
      width: 100%;margin: 0;background: linear-gradient(#111827,#374151 76%,#6b7280);
      background-clip: text;-webkit-background-clip: text;-webkit-text-fill-color: transparent;
      font-size: clamp(58px,5.95vw,85.6px);line-height: .95;font-weight: 950;
    }
    .brand-home.is-dark h1 { background: linear-gradient(#fff,#e5e7eb 72%,#94a3b8);background-clip: text;-webkit-background-clip: text; }
    .brand-home .brand-subtitle { margin: 15px 0 0;color: var(--body);font-size: clamp(25px,2.33vw,33.6px);line-height: 1.5; }
    .brand-home .tagline { margin: 5px 0 0;color: var(--muted);font-size: clamp(17px,1.42vw,20.5px);line-height: 1.62; }
    .brand-home .primary-cta {
      position: relative;overflow: hidden;width: 136px;height: 50px;display: inline-flex;align-items: center;
      justify-content: center;gap: 7px;margin-top: 51px;border-color: #25252b;border-radius: 12px;
      color: #fff;background: linear-gradient(#25252b,#141418);
      box-shadow: 0 18px 34px rgba(17,24,39,.2),inset 0 1px 0 rgba(255,255,255,.14);
      font-size: 14.4px;font-weight: 900;
    }
    .brand-home .primary-cta:hover { transform: translateY(-3px);border-color: #a78bfa;box-shadow: 0 22px 40px rgba(91,33,182,.25); }
    .brand-home.is-dark .primary-cta { border-color: #6d28d9;background: linear-gradient(#7c3aed,#5b21b6); }
    .brand-home .stats { display: grid;grid-template-columns: repeat(3,156.8px);justify-content: center;gap: 64.8px;margin-top: 44px; }
    .brand-home .stat strong {
      display: block;background: linear-gradient(#5b21b6,#a78bfa);background-clip: text;
      -webkit-background-clip: text;-webkit-text-fill-color: transparent;font-size: 40.8px;line-height: 1;font-weight: 950;
    }
    .brand-home .stat span { display: block;margin-top: 8px;color: var(--body);font-size: 13px; }

    .brand-home .features { display: grid;grid-template-columns: repeat(4,1fr);gap: 20px;padding: 0 32px 28px; }
    .brand-home .feature-card,.brand-home .model-card {
      border: 1px solid var(--line);border-radius: 18px;background: var(--card);
      box-shadow: 0 22px 44px var(--shadow),inset 0 1px 0 rgba(255,255,255,.25);
      transition: transform .26s cubic-bezier(.2,.8,.2,1),border-color .26s ease,box-shadow .26s ease,background .26s ease;
    }
    .brand-home .feature-card { min-height: 242px;padding: 26px 25px;text-align: left; }
    .brand-home .feature-card:hover,.brand-home .model-card:hover { transform: translateY(-8px);border-color: #a78bfa;box-shadow: 0 30px 58px rgba(76,29,149,.16); }
    .brand-home.is-dark .feature-card:hover,.brand-home.is-dark .model-card:hover { border-color: #7c3aed;box-shadow: 0 30px 60px rgba(0,0,0,.34),0 0 34px rgba(124,58,237,.11); }
    .brand-home .feature-icon { width: 48px;height: 48px;display: grid;place-items: center;border-radius: 14px;color: var(--purple);background: var(--purple-light); }
    .brand-home .feature-card h2 { margin: 23px 0 10px;font-size: 20px;font-weight: 850; }
    .brand-home .feature-card p { margin: 0;color: var(--body);font-size: 14px;line-height: 1.75; }
    .brand-home .families { min-height: 86px;display: flex;align-items: center;justify-content: center;gap: 34px;color: var(--body);font-size: 22px;font-weight: 850; }

    .brand-home .pricing-view { min-height: calc(100vh - 88px);padding: 26px 32px 70px; }
    .brand-home .filters { display: flex;flex-wrap: wrap;gap: 10px;padding: 15px 0 22px; }
    .brand-home .filter-button {
      min-height: 42px;display: inline-flex;align-items: center;gap: 9px;border-color: var(--line);
      border-radius: 999px;padding: 0 17px;color: var(--body);background: var(--card);font-size: 14px;font-weight: 750;
    }
    .brand-home .filter-button:hover { transform: translateY(-2px);border-color: #a78bfa; }
    .brand-home .filter-button.active { color: #fff;border-color: var(--purple);background: var(--purple); }
    .brand-home .filter-count { min-width: 23px;padding: 2px 6px;border-radius: 999px;color: var(--purple);background: var(--purple-light);font-size: 11px; }
    .brand-home .filter-button.active .filter-count { color: #4c1d95;background: #fff; }
    .brand-home .model-grid { display: grid;grid-template-columns: repeat(4,minmax(0,1fr));gap: 16px; }
    .brand-home .model-status {
      grid-column: 1/-1;min-height: 280px;display: flex;flex-direction: column;align-items: center;
      justify-content: center;gap: 13px;color: var(--body);text-align: center;
    }
    .brand-home .model-status strong { color: var(--text);font-size: 17px; }
    .brand-home .retry-button {
      min-height: 40px;border: 1px solid var(--line);border-radius: 10px;padding: 0 16px;
      color: var(--purple);background: var(--card);font-weight: 800;
    }
    .brand-home .retry-button:hover { border-color: var(--purple);transform: translateY(-2px); }
    .brand-home .model-card { min-height: 285px;padding: 23px;text-align: left; }
    .brand-home .model-head { display: flex;align-items: flex-start;justify-content: space-between;gap: 12px; }
    .brand-home .model-name { overflow-wrap: anywhere;margin: 0;font: 800 17px/1.3 ui-monospace,SFMono-Regular,Consolas,monospace; }
    .brand-home .vendor { flex: 0 0 auto;display: inline-flex;flex-direction: column;align-items: center;gap: 4px;font-size: 10px;font-weight: 850; }
    .brand-home .vendor-icon { width: 28px;height: 28px;display: grid;place-items: center;border-radius: 9px; }
    .brand-home .vendor-icon svg { width: 20px;height: 20px;display: block;color: inherit; }
    .brand-home .vendor-label { line-height: 1;text-align: center;letter-spacing: .025em; }
    .brand-home .vendor-openai { color: rgb(22,163,74); }
    .brand-home .vendor-openai .vendor-icon { background: rgb(236,253,245); }
    .brand-home .vendor-anthropic { color: rgb(234,88,12); }
    .brand-home .vendor-anthropic .vendor-icon { background: rgb(255,247,237); }
    .brand-home .vendor-gemini { color: rgb(37,99,235); }
    .brand-home .vendor-gemini .vendor-icon { background: rgb(239,246,255); }
    .brand-home .vendor-grok { color: rgb(39,39,42); }
    .brand-home .vendor-grok .vendor-icon { background: rgb(244,244,245); }
    .brand-home .cache-badge { margin-top: 14px;color: var(--body);font-size: 11px; }
    .brand-home .price-grid { display: grid;grid-template-columns: minmax(0,1fr) minmax(0,1fr);gap: 18px;margin-top: 28px; }
    .brand-home .price span { display: block;color: var(--body);font-size: 12px; }
    .brand-home .price strong { display: block;margin-top: 6px;font-size: 20px;overflow-wrap: anywhere; }
    .brand-home .price small { color: var(--body);font-size: 10px; }
    .brand-home .model-card hr { margin: 20px 0;border: 0;border-top: 1px solid var(--line); }
    .brand-home .cache-row { display: flex;align-items: center;justify-content: space-between;gap: 10px;color: var(--body);font-size: 12px; }
    .brand-home .cache-row + .cache-row { margin-top: 8px; }
    .brand-home .cache-row > span { flex-shrink: 0; }
    .brand-home .cache-row strong { min-width: 0;color: var(--text);overflow-wrap: anywhere;text-align: right; }
    .brand-home .image-card { grid-column: span 2;min-height: 285px; }
    .brand-home .image-price-grid { display: grid;grid-template-columns: repeat(3,1fr);gap: 10px;margin-top: 20px; }
    .brand-home .image-price {
      min-width: 0;border: 1px solid var(--line);border-radius: 12px;padding: 17px 12px;
      background: var(--purple-light);text-align: center;
    }
    .brand-home .image-price span { display: block;color: var(--body);font-size: 12px;font-weight: 750; }
    .brand-home .image-price strong { display: block;margin-top: 8px;color: var(--purple);font-size: 24px;overflow-wrap: anywhere; }
    .brand-home .image-price small { display: block;margin-top: 4px;color: var(--body);font-size: 10px; }
    .brand-home .image-note { margin: 18px 0 0;color: var(--body);font-size: 12px;line-height: 1.7; }
    .brand-home .footer { padding: 28px 20px;color: var(--body);text-align: center;font-size: 13px; }

    @media (max-width:1050px) {
      .brand-home .features { grid-template-columns: repeat(2,1fr); }
      .brand-home .model-grid { grid-template-columns: repeat(3,minmax(0,1fr)); }
      .brand-home .image-card { grid-column: span 2; }
    }
    @media (max-width:760px) {
      .brand-home .header { height: 72px;padding: 16px; }
      .brand-home .nav { height: 40px;grid-template-columns: 1fr auto; }
      .brand-home .desktop-nav,.brand-home .auth-button { display: none; }
      .brand-home .menu-button { display: grid; }
      .brand-home .view { width: 100%; }
      .brand-home .hero { min-height: 530px;padding: 44px 20px 42px;justify-content: center; }
      .brand-home .hero-glow { width: min(420px,100vw); }
      .brand-home h1 { font-size: clamp(54px,17vw,72px); }
      .brand-home .brand-subtitle { font-size: 26px; }
      .brand-home .tagline { margin-top: 10px;font-size: 17px; }
      .brand-home .stats { width: 100%;grid-template-columns: repeat(3,1fr);gap: 0; }
      .brand-home .stat { padding: 0 6px;border-right: 1px solid var(--line); }
      .brand-home .stat:last-child { border-right: 0; }
      .brand-home .stat strong { font-size: 27px; }
      .brand-home .stat span { font-size: 11px; }
      .brand-home .features { grid-template-columns: 1fr;gap: 12px;padding: 0 16px 24px; }
      .brand-home .families { flex-wrap: wrap;gap: 20px;padding: 24px 16px; }
      .brand-home .pricing-view { padding: 18px 16px 70px; }
      .brand-home .model-grid { grid-template-columns: 1fr; }
      .brand-home .image-card { grid-column: auto; }
      .brand-home .image-price-grid { gap: 7px; }
      .brand-home .image-price { padding: 15px 8px; }
    }
    @media (prefers-reduced-motion:reduce) {
      .brand-home { scroll-behavior: auto; }
      .brand-home::before,.brand-home::after,.brand-home *,.brand-home *::before,.brand-home *::after { animation-duration: .01ms !important;animation-iteration-count: 1 !important;transition-duration: .01ms !important; }
    }

.brand-home .logo img { width: 100%; height: 100%; border-radius: inherit; object-fit: contain; }
.brand-home .vendor { max-width: 45%; overflow-wrap: anywhere; }

/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: 420px;
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 14px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(30, 41, 59, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a78bfa;
}
.code-url {
  color: #14b8a6;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Blinking Cursor */
.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(20, 184, 166, 0.2),
    0 0 40px rgba(20, 184, 166, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
</style>
