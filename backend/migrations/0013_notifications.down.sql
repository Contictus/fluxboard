-- 0013 down — drop Phase 5 notification/stats tables (RLS in 0014 dropped first).
DROP TABLE IF EXISTS project_stats_daily;
DROP TABLE IF EXISTS notification_prefs;
DROP TABLE IF EXISTS notifications;
