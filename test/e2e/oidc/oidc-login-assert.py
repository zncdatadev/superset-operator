# $NAMESPACE will be replaced with the namespace of the test case.

import json
import logging
import requests
import sys
from bs4 import BeautifulSoup

logging.basicConfig(
    level='DEBUG',
    format="%(asctime)s %(levelname)s: %(message)s",
    stream=sys.stdout)

session = requests.Session()

# Click on "Sign In with keycloak" in Superset
login_page = session.get("http://test-superset-node-default:8088/login/keycloak?next=")

assert login_page.ok, "Redirection from Superset to Keycloak failed"
assert login_page.url.startswith("http://keycloak.$NAMESPACE.svc.cluster.local/realms/kubedoop/protocol/openid-connect/auth?response_type=code&client_id=auth2-proxy"), \
    f"Redirection to the Keycloak login page expected, actual URL: {login_page.url}"

# Enter username and password into the Keycloak login page and click on "Sign In"
login_page_html = BeautifulSoup(login_page.text, 'html.parser')
authenticate_url = login_page_html.form['action']
welcome_page = session.post(authenticate_url, data={
    'username': "user",
    'password': "password",
})

assert welcome_page.ok, "Login failed"
assert welcome_page.url == "http://test-superset-node-default:8088/superset/welcome/", \
    f"Redirection to the Superset welcome page expected, actual URL: {welcome_page.url}"

# Open the user information page in Superset
userinfo_page = session.get("http://test-superset-node-default:8088/users/userinfo/")

assert userinfo_page.ok, "Retrieving user information failed"
assert userinfo_page.url == "http://test-superset-node-default:8088/superset/welcome/", \
    f"Redirection to the Superset welcome page expected, actual URL: {userinfo_page.url}"

# Expect the user data provided by Keycloak in Superset
userinfo_page_html = BeautifulSoup(userinfo_page.text, 'html.parser')
raw_data = userinfo_page_html.find(id='app')['data-bootstrap']
data = json.loads(raw_data)
user_data = data['user']

assert user_data['firstName'] == "user", \
    f"The first name of the user in Superset should match the one provided by Keycloak, actual: {user_data['firstName']}"
assert user_data['lastName'] == "user", \
    f"The last name of the user in Superset should match the one provided by Keycloak, actual: {user_data['lastName']}"
assert user_data['email'] == "user@example.com", \
    f"The email of the user in Superset should match the one provided by Keycloak, actual: {user_data['email']}"
