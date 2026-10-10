ALTER TABLE income_cap_diversions
  ADD COLUMN commission_kind VARCHAR(40) NOT NULL DEFAULT 'direct_team',
  DROP INDEX cap_diversion_identity,
  ADD UNIQUE KEY cap_diversion_identity (user_id,purchase_intent_id,commission_kind);
