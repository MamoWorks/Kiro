/**
 * 刷新 Kiro Token (桌面端 API)
 * 只需 refreshToken，无需其他参数
 * 用法: node refresh-token.js <refreshToken>
 */

const KIRO_DESKTOP_API = 'https://prod.us-east-1.auth.desktop.kiro.dev';

/**
 * 调用桌面端 RefreshToken API
 * @param {string} refreshToken - RefreshToken
 * @returns {Promise<{accessToken: string, refreshToken: string, expiresIn: number, profileArn: string}>}
 */
async function refreshTokenDesktop(refreshToken) {
    const url = `${KIRO_DESKTOP_API}/refreshToken`;

    console.log('请求参数:');
    console.log(JSON.stringify({
        url,
        refreshToken: refreshToken.substring(0, 30) + '...'
    }, null, 2));

    const response = await fetch(url, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        },
        body: JSON.stringify({ refreshToken })
    });

    const data = await response.json();

    if (!response.ok) {
        if (response.status === 423 || JSON.stringify(data).includes('AccountSuspendedException')) {
            throw new Error('BANNED: 账号已被封禁');
        }
        throw new Error(`RefreshToken failed (${response.status}): ${JSON.stringify(data)}`);
    }

    return data;
}

/**
 * 主函数
 */
async function main() {
    const args = process.argv.slice(2);

    if (args.length < 1) {
        console.log('用法: node refresh-token.js <refreshToken>');
        console.log('');
        console.log('参数:');
        console.log('  refreshToken - RefreshToken (只需这一个参数)');
        console.log('');
        console.log('示例:');
        console.log('  node refresh-token.js "aorAAAAAGmxOC8i4PO-b3RUn..."');
        process.exit(1);
    }

    const [refreshToken] = args;

    console.log('\n========== RefreshToken (桌面端 API) ==========\n');

    const result = await refreshTokenDesktop(refreshToken);

    console.log('\n========== 刷新结果 ==========\n');
    console.log(JSON.stringify({
        accessToken: result.accessToken ? result.accessToken.substring(0, 50) + '...' : null,
        refreshToken: result.refreshToken ? result.refreshToken.substring(0, 30) + '...' : null,
        expiresIn: result.expiresIn,
        profileArn: result.profileArn
    }, null, 2));

    console.log('\n========== 完整新 Token ==========\n');
    console.log(JSON.stringify({
        accessToken: result.accessToken,
        refreshToken: result.refreshToken,
        expiresIn: result.expiresIn,
        profileArn: result.profileArn,
        refreshedAt: new Date().toISOString()
    }, null, 2));
}

main().catch(err => {
    console.error('\n错误:', err.message);
    process.exit(1);
});
