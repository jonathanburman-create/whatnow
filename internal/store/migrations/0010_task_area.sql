-- The part of life a task belongs to, such as "huji" or "home". Null until one is picked.
ALTER TABLE tasks ADD COLUMN area TEXT;
