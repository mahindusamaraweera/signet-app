-- Device action challenges bind one device identity; old email codes stay valid.
ALTER TABLE withdrawal_security_challenges ADD COLUMN target CHAR(64) NULL;
