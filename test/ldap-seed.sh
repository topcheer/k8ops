#!/bin/bash
# Seed LDAP with test users for k8ops testing
set -e

echo "Waiting for OpenLDAP..."
for i in $(seq 1 30); do
  if ldapsearch -H ldap://openldap:389 -x -D cn=admin,dc=k8ops,dc=test -w admin -b dc=k8ops,dc=test -s base dn 2>/dev/null | grep -q "dn:"; then
    echo "OpenLDAP is ready"
    break
  fi
  sleep 1
done

echo "Seeding LDAP..."

ldapadd -H ldap://openldap:389 -x -D cn=admin,dc=k8ops,dc=test -w admin <<'LDIF'
dn: ou=people,dc=k8ops,dc=test
objectClass: organizationalUnit
ou: people

dn: ou=groups,dc=k8ops,dc=test
objectClass: organizationalUnit
ou: groups

dn: uid=viewer1,ou=people,dc=k8ops,dc=test
objectClass: inetOrgPerson
cn: Test Viewer
sn: Viewer
uid: viewer1
mail: viewer1@k8ops.test
userPassword: viewer123

dn: uid=operator1,ou=people,dc=k8ops,dc=test
objectClass: inetOrgPerson
cn: Test Operator
sn: Operator
uid: operator1
mail: operator1@k8ops.test
userPassword: operator123

dn: uid=ldapadmin,ou=people,dc=k8ops,dc=test
objectClass: inetOrgPerson
cn: LDAP Admin
sn: Admin
uid: ldapadmin
mail: ldapadmin@k8ops.test
userPassword: admin123

dn: cn=k8ops-viewers,ou=groups,dc=k8ops,dc=test
objectClass: groupOfNames
cn: k8ops-viewers
member: uid=viewer1,ou=people,dc=k8ops,dc=test

dn: cn=k8ops-operators,ou=groups,dc=k8ops,dc=test
objectClass: groupOfNames
cn: k8ops-operators
member: uid=operator1,ou=people,dc=k8ops,dc=test
LDIF

echo "LDAP seeded successfully:"
echo "  Users: viewer1/viewer123, operator1/operator123, ldapadmin/admin123"
echo "  Groups: k8ops-viewers, k8ops-operators"
