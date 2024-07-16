import requests
import os
import hashlib
from pathlib import Path
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.cron import CronTrigger


class JamfProSync:
    def __init__(self, api_endpoint, client_id, client_secret, local_folder):
        self.jamf_api_endpoint = api_endpoint
        self.jamf_api_client_id = client_id
        self.jamf_api_client_secret = client_secret
        self.local_folder = Path(local_folder)

    def authenticate_jamf_api(self):
        headers = {'Content-Type': 'application/x-www-form-urlencoded'}
        data = {
            'client_id': self.jamf_api_client_id,
            'client_secret': self.jamf_api_client_secret,
            'grant_type': 'client_credentials'
        }
        response = requests.post(f'{self.jamf_api_endpoint}/api/oauth/token', headers=headers, data=data)
        response.raise_for_status()
        return response.json()['access_token']

    def fetch_packages(self, token):
        headers = {'Authorization': f'Bearer {token}'}
        response = requests.get(f'{self.jamf_api_endpoint}/api/v1/packages?page=0&page-size=100&sort=id%3Aasc', headers=headers)
        response.raise_for_status()
        return response.json()['results']

    def download_file(self, token, file_name, local_path):
        url = f'{self.jamf_api_endpoint}/api/v1/jcds/files/{file_name}'
        headers = {'Authorization': f'Bearer {token}'}
        response = requests.get(url, headers=headers)
        response.raise_for_status()
        download_uri = response.json()['uri']
        download_response = requests.get(download_uri, stream=True)
        download_response.raise_for_status()
        with open(local_path, 'wb') as f:
            for chunk in download_response.iter_content(chunk_size=8192):
                if chunk:
                    f.write(chunk)
        print(f"Downloaded {file_name} to {local_path}")

    def sync(self):
        token = self.authenticate_jamf_api()
        packages = self.fetch_packages(token)
        package_filenames = {package['fileName'] for package in packages}
        for package in packages:
            local_file_path = self.local_folder / package['fileName']
            remote_md5 = package['md5']
            if local_file_path.exists():
                local_md5 = self.md5(local_file_path)
                print(f"Package '{package['packageName']}' - Local MD5: {local_md5}, Remote MD5: {remote_md5}")
                if local_md5 != remote_md5:
                    print(f"Updating file {local_file_path} for package '{package['packageName']}'")
                    self.download_file(token, package['fileName'], local_file_path)
            else:
                print(f"Downloading new file {local_file_path} for package '{package['packageName']}'")
                self.download_file(token, package['fileName'], local_file_path)
        for existing_file in self.local_folder.iterdir():
            if existing_file.name not in package_filenames and not existing_file.name.startswith('.'):
                print(f"Deleting outdated file {existing_file}")
                try:
                    existing_file.unlink()
                except FileNotFoundError as e:
                    print(f"Error deleting file {existing_file}: {e}")

    @staticmethod
    def md5(file_path):
        hash_md5 = hashlib.md5()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                hash_md5.update(chunk)
        return hash_md5.hexdigest()


def main():
    client_id = os.getenv('JAMF_CLIENT_ID', '')
    client_secret = os.getenv('JAMF_CLIENT_SECRET', '')
    local_folder = '/packages'
    api_endpoint = os.getenv('JAMF_CLIENT_SECRET', '')
    cron_schedule = os.getenv('SYNC_SCHEDULE', '0 0 * * *')
    sync_now = os.getenv('SYNC_NOW', 'false').lower() == 'true'
    sync = JamfProSync(api_endpoint, client_id, client_secret, local_folder)
    if sync_now:
        sync.sync()
    else:
        scheduler = BackgroundScheduler()
        scheduler.add_job(sync.sync, CronTrigger.from_crontab(cron_schedule))
        scheduler.start()
        try:
            import time
            while True:
                time.sleep(10)
        except (KeyboardInterrupt, SystemExit):
            scheduler.shutdown()


if __name__ == '__main__':
    main()
