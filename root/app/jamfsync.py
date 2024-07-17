import requests
import os
import hashlib
from pathlib import Path
from datetime import datetime, timedelta, timezone
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.cron import CronTrigger
import logging

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')


class JCDSSync:
    def __init__(self, api_endpoint, client_id, client_secret, local_folder):
        self.jamf_api_endpoint = api_endpoint
        self.jamf_api_client_id = client_id
        self.jamf_api_client_secret = client_secret
        self.local_folder = Path(local_folder)
        self.access_token = None
        self.token_expiry = datetime.now(timezone.utc)
        logging.info('Initialized JCDSSync')

    def authenticate_jamf_api(self):
        logging.info('Authenticating with Jamf Pro API...')
        headers = {'Content-Type': 'application/x-www-form-urlencoded'}
        data = {
            'client_id': self.jamf_api_client_id,
            'client_secret': self.jamf_api_client_secret,
            'grant_type': 'client_credentials'
        }
        response = requests.post(f'{self.jamf_api_endpoint}/api/oauth/token', headers=headers, data=data)
        response.raise_for_status()
        token_data = response.json()
        self.access_token = token_data['access_token']
        self.token_expiry = datetime.now(timezone.utc) + timedelta(seconds=token_data['expires_in'])
        logging.info('Successfully authenticated and received new access token.')

    def check_token(self):
        if datetime.now(timezone.utc) >= self.token_expiry:
            logging.info('Access token expired, re-authenticating...')
            self.authenticate_jamf_api()

    def fetch_packages(self):
        self.check_token()
        logging.info('Fetching package list from Jamf Pro...')
        headers = {'Authorization': f'Bearer {self.access_token}'}
        response = requests.get(f'{self.jamf_api_endpoint}/api/v1/packages?page=0&page-size=100&sort=id%3Aasc', headers=headers)
        response.raise_for_status()
        return response.json()['results']

    def download_file(self, file_name, local_path):
        self.check_token()
        logging.info(f'Downloading file: {file_name}')
        url = f'{self.jamf_api_endpoint}/api/v1/jcds/files/{file_name}'
        headers = {'Authorization': f'Bearer {self.access_token}'}
        response = requests.get(url, headers=headers)
        response.raise_for_status()
        download_uri = response.json()['uri']
        download_response = requests.get(download_uri, stream=True)
        download_response.raise_for_status()
        with open(local_path, 'wb') as f:
            for chunk in download_response.iter_content(chunk_size=8192):
                if chunk:
                    f.write(chunk)
        logging.info(f'Successfully downloaded {file_name} to {local_path}')

    def sync(self):
        logging.info('Starting synchronization process...')
        self.check_token()
        packages = self.fetch_packages()
        package_filenames = {package['fileName'] for package in packages}
        for package in packages:
            local_file_path = self.local_folder / package['fileName']
            remote_md5 = package['md5']
            if local_file_path.exists():
                local_md5 = self.md5(local_file_path)
                if local_md5 != remote_md5:
                    logging.info(f'MD5 mismatch, updating file: {package["fileName"]}')
                    self.download_file(package['fileName'], local_file_path)
            else:
                logging.info(f'Downloading new file: {package["fileName"]}')
                self.download_file(package['fileName'], local_file_path)
        for existing_file in self.local_folder.iterdir():
            if existing_file.name not in package_filenames and not existing_file.name.startswith('.'):
                logging.info(f'Deleting outdated file: {existing_file.name}')
                existing_file.unlink()

    @staticmethod
    def md5(file_path):
        hash_md5 = hashlib.md5()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                hash_md5.update(chunk)
        return hash_md5.hexdigest()


def main():
    logging.info('Script started')
    client_id = os.getenv('JAMF_CLIENT_ID', '')
    client_secret = os.getenv('JAMF_CLIENT_SECRET', '')
    local_folder = '/packages'
    api_endpoint = os.getenv('JAMF_URL', '')
    cron_schedule = os.getenv('SYNC_SCHEDULE', '0 0 * * *')
    sync_now = os.getenv('SYNC_NOW', 'false').lower() == 'true'
    sync = JCDSSync(api_endpoint, client_id, client_secret, local_folder)
    if sync_now:
        logging.info('Running sync immediately due to SYNC_NOW setting')
        sync.sync()
    else:
        scheduler = BackgroundScheduler()
        scheduler.add_job(sync.sync, CronTrigger.from_crontab(cron_schedule))
        scheduler.start()
        logging.info(f'Scheduled sync with cron: {cron_schedule}')
        try:
            import time
            while True:
                time.sleep(10)
        except (KeyboardInterrupt, SystemExit):
            logging.info('Scheduler shutdown initiated')
            scheduler.shutdown()


if __name__ == '__main__':
    main()
