ALTER TABLE placement_correction_holds
 ADD COLUMN status VARCHAR(30) NOT NULL DEFAULT 'held',
 ADD COLUMN completion JSON NULL;
