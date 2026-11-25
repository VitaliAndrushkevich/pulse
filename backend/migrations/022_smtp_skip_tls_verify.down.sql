UPDATE monitors
SET settings = settings - 'skip_tls_verify'
WHERE type = 'smtp';
