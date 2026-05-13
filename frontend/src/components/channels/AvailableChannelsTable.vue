<template>
  <div class="card overflow-hidden">
    <table class="w-full table-fixed border-collapse text-sm">
      <thead>
        <tr class="border-b border-gray-100 bg-gray-50/50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400">
          <th class="w-[180px] px-4 py-3 text-center">{{ columns.name }}</th>
          <th class="w-[220px] px-4 py-3 text-left">{{ columns.description }}</th>
          <th class="px-4 py-3 text-left">{{ columns.groups }}</th>
          <th class="px-4 py-3 text-left">{{ columns.supportedModels }}</th>
        </tr>
      </thead>
      <tbody v-if="loading">
        <tr>
          <td colspan="4" class="py-10 text-center">
            <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
          </td>
        </tr>
      </tbody>
      <tbody v-else-if="rows.length === 0">
        <tr>
          <td colspan="4" class="py-12 text-center">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
          </td>
        </tr>
      </tbody>
      <tbody v-else>
        <tr
          v-for="(channel, idx) in rows"
          :key="`${channel.name}-${idx}`"
          class="border-b border-gray-100 transition-colors hover:bg-gray-50/40 last:border-b-0 dark:border-dark-700 dark:hover:bg-dark-800/40"
        >
          <td class="px-4 py-3 text-center align-middle font-medium text-gray-900 dark:text-white">
            {{ channel.name }}
          </td>

          <td class="px-4 py-3 align-middle text-xs text-gray-500 dark:text-gray-400">
            <template v-if="channel.description">{{ channel.description }}</template>
            <span v-else class="text-gray-400">-</span>
          </td>

          <td class="px-4 py-3 align-top">
            <div class="flex flex-col gap-1.5">
              <div
                v-if="exclusiveGroups(channel).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-purple-600 dark:text-purple-400"
                  :title="t('availableChannels.exclusiveTooltip')"
                >
                  <Icon name="shield" size="xs" class="h-3 w-3" />
                  {{ t('availableChannels.exclusive') }}
                </span>
                <GroupBadge
                  v-for="g in exclusiveGroups(channel)"
                  :key="`ex-${g.id}`"
                  :name="g.name"
                  :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                  :rate-multiplier="g.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[g.id] ?? null"
                  always-show-rate
                />
              </div>
              <div
                v-if="publicGroups(channel).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-gray-500 dark:text-gray-400"
                  :title="t('availableChannels.publicTooltip')"
                >
                  <Icon name="globe" size="xs" class="h-3 w-3" />
                  {{ t('availableChannels.public') }}
                </span>
                <GroupBadge
                  v-for="g in publicGroups(channel)"
                  :key="`pub-${g.id}`"
                  :name="g.name"
                  :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                  :rate-multiplier="g.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[g.id] ?? null"
                  always-show-rate
                />
              </div>
              <span v-if="channel.groups.length === 0" class="text-xs text-gray-400">-</span>
            </div>
          </td>

          <td class="px-4 py-3 align-top">
            <div class="flex flex-wrap gap-1">
              <SupportedModelChip
                v-for="m in channel.supported_models"
                :key="m.name"
                :model="m"
                :pricing-key-prefix="pricingKeyPrefix"
                :no-pricing-label="noPricingLabel"
                :show-platform="false"
              />
              <span v-if="channel.supported_models.length === 0" class="text-xs text-gray-400">
                {{ noModelsLabel }}
              </span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import SupportedModelChip from './SupportedModelChip.vue'
import type { UserAvailableChannel, UserAvailableGroup } from '@/api/channels'
import type { SubscriptionType } from '@/types'

defineProps<{
  columns: {
    name: string
    description: string
    groups: string
    supportedModels: string
  }
  rows: UserAvailableChannel[]
  loading: boolean
  pricingKeyPrefix: string
  noPricingLabel: string
  noModelsLabel: string
  emptyLabel: string
  userGroupRates: Record<number, number>
}>()

const { t } = useI18n()

function exclusiveGroups(channel: UserAvailableChannel): UserAvailableGroup[] {
  return channel.groups.filter((g) => g.is_exclusive)
}

function publicGroups(channel: UserAvailableChannel): UserAvailableGroup[] {
  return channel.groups.filter((g) => !g.is_exclusive)
}
</script>
