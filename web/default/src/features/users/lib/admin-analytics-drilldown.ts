import type {
  AdminAnalyticsUsersDrilldownEnvelopeResponse,
  AdminAnalyticsUsersDrilldownResponse,
} from '../types'

export function normalizeAdminAnalyticsUsersDrilldownResponse(
  response: AdminAnalyticsUsersDrilldownEnvelopeResponse
): AdminAnalyticsUsersDrilldownResponse {
  return {
    success: response.success,
    message: response.message,
    data: response.data?.data,
  }
}
