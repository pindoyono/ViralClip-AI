import type { ClipAnalytics, SocialPlatform } from "@viralclip/shared-types";

export interface EngagementMetrics {
  views: number;
  likes: number;
  comments: number;
  shares: number;
  saves: number;
  reach: number;
  engagement_rate: number;
}

export function calculateEngagementRate(metrics: Omit<EngagementMetrics, "engagement_rate">): number {
  const { views, likes, comments, shares, saves } = metrics;
  if (views === 0) return 0;
  const engagements = likes + comments + shares + saves;
  return Math.round((engagements / views) * 10000) / 100;
}

export function aggregateAnalytics(records: ClipAnalytics[]): EngagementMetrics {
  const totals = records.reduce(
    (acc, r) => ({
      views: acc.views + r.views,
      likes: acc.likes + r.likes,
      comments: acc.comments + r.comments,
      shares: acc.shares + r.shares,
      saves: acc.saves + r.saves,
      reach: acc.reach + r.reach,
      engagement_rate: 0,
    }),
    { views: 0, likes: 0, comments: 0, shares: 0, saves: 0, reach: 0, engagement_rate: 0 }
  );
  totals.engagement_rate = calculateEngagementRate(totals);
  return totals;
}

export function getTopPlatform(records: ClipAnalytics[]): SocialPlatform | null {
  if (!records.length) return null;
  const byPlatform = records.reduce((acc, r) => {
    acc[r.platform] = (acc[r.platform] || 0) + r.views;
    return acc;
  }, {} as Record<string, number>);
  const top = Object.entries(byPlatform).sort((a, b) => b[1] - a[1])[0];
  return top ? (top[0] as SocialPlatform) : null;
}
