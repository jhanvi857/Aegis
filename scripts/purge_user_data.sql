-- Purge all user accounts, API keys, audit logs, and associated topologies
TRUNCATE TABLE 
    topology_edges, 
    topology_services, 
    topology_versions, 
    recovery_executions, 
    audit_logs, 
    api_keys, 
    users 
CASCADE;

