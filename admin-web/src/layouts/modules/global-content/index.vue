<script setup lang="ts">
import { computed } from 'vue';
import { LAYOUT_SCROLL_EL_ID } from '@sa/materials';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { useRouteStore } from '@/store/modules/route';
import { useTabStore } from '@/store/modules/tab';
import { useRoute } from 'vue-router';
import { serverScopedRoutes, useServerScopeStore } from '@/store/modules/server-scope';

defineOptions({
  name: 'GlobalContent'
});

interface Props {
  /** Show padding for content */
  showPadding?: boolean;
}

withDefaults(defineProps<Props>(), {
  showPadding: true
});

const appStore = useAppStore();
const themeStore = useThemeStore();
const routeStore = useRouteStore();
const tabStore = useTabStore();
const route = useRoute();
const scope = useServerScopeStore();
const scoped = computed(() => serverScopedRoutes.has(String(route.name)));

const transitionName = computed(() => (themeStore.page.animate ? themeStore.page.animateMode : ''));

function resetScroll() {
  const el = document.querySelector(`#${LAYOUT_SCROLL_EL_ID}`);

  el?.scrollTo({ left: 0, top: 0 });
}
</script>

<template>
  <div v-if="scoped && !scope.currentServer" class="p-24px">
    <NCard :bordered="false">
      <NEmpty :description="scope.loading ? '正在加载区服' : '请在顶栏选择运营区服'">
        <template #extra><NButton :loading="scope.loading" @click="scope.loadServers">刷新区服列表</NButton></template>
      </NEmpty>
      <NAlert v-if="scope.error" class="mt-16px" type="error" :title="scope.error" />
    </NCard>
  </div>
  <RouterView v-else v-slot="{ Component, route: childRoute }">
    <Transition
      :name="transitionName"
      mode="out-in"
      @before-leave="appStore.setContentXScrollable(true)"
      @after-leave="resetScroll"
      @after-enter="appStore.setContentXScrollable(false)"
    >
      <KeepAlive :include="routeStore.cacheRoutes" :exclude="routeStore.excludeCacheRoutes">
        <component
          :is="Component"
          v-if="appStore.reloadFlag"
          :key="`${tabStore.getTabIdByRoute(childRoute)}${scoped ? `:server:${scope.serverId}` : ''}`"
          :class="{ 'p-16px': showPadding }"
          class="flex-grow bg-layout transition-300"
        />
      </KeepAlive>
    </Transition>
  </RouterView>
</template>

<style></style>
