DO $$
BEGIN
    RAISE EXCEPTION 'migration 10 is irreversible because obsolete note, search, AI, and podcast data was removed';
END
$$;
