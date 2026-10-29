CREATE TABLE app_roles (
 app_id text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
 id text NOT NULL,
 name text NOT NULL,
 role_key text NOT NULL,
 description text NOT NULL DEFAULT '',
 is_default boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(app_id,id), UNIQUE(app_id,role_key)
);
CREATE UNIQUE INDEX app_roles_one_default ON app_roles(app_id) WHERE is_default;
INSERT INTO app_roles(app_id,id,name,role_key,description,is_default)
SELECT id,'admin','管理员','admin','项目管理员',false FROM apps
UNION ALL SELECT id,'user','用户','user','项目普通用户',true FROM apps;
CREATE FUNCTION initialize_app_roles() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO app_roles(app_id,id,name,role_key,description,is_default) VALUES
 (NEW.id,'admin','管理员','admin','项目管理员',false),
 (NEW.id,'user','用户','user','项目普通用户',true);
 RETURN NEW;
END;
$$;
CREATE TRIGGER initialize_app_roles AFTER INSERT ON apps FOR EACH ROW EXECUTE FUNCTION initialize_app_roles();
ALTER TABLE app_users ADD COLUMN role_id text;
UPDATE app_users SET role_id='user';
ALTER TABLE app_users ALTER COLUMN role_id SET NOT NULL;
ALTER TABLE app_users ADD CONSTRAINT app_users_role_fk FOREIGN KEY(app_id,role_id) REFERENCES app_roles(app_id,id);
CREATE FUNCTION assign_default_app_role() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.role_id IS NULL THEN
  PERFORM 1 FROM apps WHERE id=NEW.app_id FOR SHARE;
  SELECT id INTO NEW.role_id FROM app_roles WHERE app_id=NEW.app_id AND is_default;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER assign_default_app_role BEFORE INSERT ON app_users FOR EACH ROW EXECUTE FUNCTION assign_default_app_role();
