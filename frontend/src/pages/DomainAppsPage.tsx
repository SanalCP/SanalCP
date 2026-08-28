import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Kurulu = {
  tur: string; ad: string; dizin: string; surum: string; son_surum: string
  durum: 'guncel' | 'eski' | 'bilinmiyor'; kurulum_tarihi: string
  site_url: string; admin_url: string
}
type FormAlan = { anahtar: string; etiket: string; tur: 'text' | 'email' | 'password'; zorunlu: boolean; yer_tutucu?: string }
type TurBilgi = { slug: string; ad: string; form_alanlari: FormAlan[] }
type Sonuc = {
  tur: string; site_url: string; admin_url: string
  admin_kullanici: string; admin_parola: string; surum: string; ekstra?: Record<string, string>
}

// Simple-Icons (CC0) — https://simpleicons.org
const ICONS: Record<string, string> = {
  wordpress: 'M21.469 6.825c.84 1.537 1.318 3.3 1.318 5.175 0 3.979-2.156 7.456-5.363 9.325l3.295-9.527c.615-1.54.82-2.771.82-3.864 0-.405-.026-.78-.07-1.11m-7.981.105c.647-.03 1.232-.105 1.232-.105.582-.075.514-.93-.067-.899 0 0-1.755.135-2.88.135-1.064 0-2.85-.15-2.85-.15-.585-.03-.661.855-.075.885 0 0 .54.061 1.125.09l1.68 4.605-2.37 7.08L5.354 6.9c.649-.03 1.234-.1 1.234-.1.585-.075.516-.93-.065-.896 0 0-1.746.138-2.874.138-.2 0-.438-.008-.69-.015C4.911 3.15 8.235 1.215 12 1.215c2.809 0 5.365 1.072 7.286 2.833-.046-.003-.091-.009-.141-.009-1.06 0-1.812.923-1.812 1.914 0 .89.513 1.643 1.06 2.531.411.72.89 1.643.89 2.977 0 .915-.354 1.994-.821 3.479l-1.075 3.585-3.9-11.61.001.014zM12 22.784c-1.059 0-2.081-.153-3.048-.437l3.237-9.406 3.315 9.087c.024.053.05.101.078.149-1.12.393-2.325.609-3.582.609M1.211 12c0-1.564.336-3.05.935-4.39L7.29 21.709C3.694 19.96 1.212 16.271 1.211 12M12 0C5.385 0 0 5.385 0 12s5.385 12 12 12 12-5.385 12-12S18.615 0 12 0',
  prestashop: 'M11.558 1.034C5.174 1.034 0 6.21 0 12.592c0 1.258.201 2.47.574 3.597l.002-.007a12.415 12.415 0 01.53-1.787l.011-.03c.085-.222.179-.442.277-.66l.084-.181c.08-.171.165-.34.253-.507.036-.068.07-.136.108-.203.02-.038.044-.073.064-.11.094-.166.19-.332.29-.493l.075-.114c.125-.195.256-.386.393-.573l.035-.05c.144-.193.295-.38.451-.563l.1-.118c.155-.177.315-.35.481-.517l.099-.097a10.321 10.321 0 01.546-.503c.74-2.48 3.005-4.285 5.686-4.285 1.079 0 2.152.31 3.071.873a6.017 6.017 0 012.211 2.407l.007.015.04.074v.003l.004.002a9.925 9.925 0 011.567 1.198c.04.037.081.071.12.109.002 0 .006.005.007.006l-.002-.006-.001-.004v-.003l.042-.084c.377-2.384 1.43-4.102 2.67-4.102.934 0 1.762.975 2.276 2.476l.005.016.001.002c.145.158.287.331.424.521l.007.01.021.067-.02-.078c-1.542-4.569-5.863-7.857-10.952-7.857zM9.927 5.477C7.586 5.52 5.34 7.132 4.574 9.365l-.012.034a10.14 10.14 0 011.315-.895c2.806-1.656 6.479-1.646 9.278.016-.895-1.653-2.631-2.819-4.5-3.004a5.14 5.14 0 00-.728-.039zm9.834.5a1.36 1.36 0 00-.39.067c-1.265.562-1.719 2.073-2.031 3.303l-.016.072c.365-.62.808-1.215 1.396-1.642.835-.687 2.105-.655 2.916.053.308.326.141.008.031-.22-.342-.75-1.025-1.653-1.906-1.634zM21.67 7.98zm-9.32.335l-1.07 3.27-.002.005-.006.002-4.498 1.112h-.009l4.456 1.087c.105.11.227.205.36.28h.002c.042.024.085.045.129.065l.01.005c.041.018.083.033.126.047l.021.008c.04.013.08.023.12.032l.033.008a1.677 1.677 0 00.318.033 1.546 1.546 0 001.43-.948c.08-.186.123-.39.123-.604v-.011l-.001-.012c-.001-.054-.004-.107-.01-.16l-.001-.002a1.506 1.506 0 00-.026-.153l-.001-.004a1.511 1.511 0 00-.096-.288v-.003a1.521 1.521 0 00-.348-.49v-.003zm3.148.626c.048 1.008.036 2.046-.1 3.057-.17 2.018-1.19 3.798-1.972 5.616l-.03.08-.035.086c1.51-1.522 3.17-3.04 3.969-5.082.383-.636.118-1.342-.115-1.976-.17-.877-1.069-1.278-1.717-1.781zm6.172.572l-.588 2.688a1.764 1.764 0 00-.047.2c-.002.02-.007.04-.01.06a1.76 1.76 0 00-.016.222l-.002.031h.003c0 .628.297 1.136.663 1.137a.41.41 0 00.182-.045l.027-.015a.537.537 0 00.07-.047c.013-.01.024-.022.036-.033a.752.752 0 00.137-.168l.03-.054a1.23 1.23 0 00.052-.108l.017-.04c.02-.053.038-.108.053-.166l.002-.002.001-.003.404-.451-.407-.456v.001l-.02-.063zm-4.381.856c.69 1.716.85 3.707.091 5.43-.49 1.368-1.587 2.463-1.874 3.905.73.115 1.468.176 2.21.186 2.166.029 4.332-.42 6.284-1.365-2.04-2.869-4.121-5.755-6.711-8.156zm-4.948.977a.583.583 0 110 1.166.583.583 0 010-1.166zm9.352.37c.138 0 .249.19.249.426s-.111.426-.249.426c-.137 0-.248-.19-.248-.426 0-.235.11-.426.248-.426zm-4.044.184c-.016.112-.033.209-.05.29l-.006.023c-.01.05-.022.094-.033.128-.48 1.417-1.275 2.52-2.36 3.697-.147.16-.301.32-.459.484a58.883 58.883 0 01-1.196 1.205c-.112.11-.259.261-.425.436-.103.287-.22.61-.318.95-.044-.016-.086-.031-.131-.049-2.108-.815-3.519-1.904-3.519-1.904s1.086 1.414 2.915 2.74c.177.129.351.24.522.339-.075 1.194.452 2.34 2.83 2.682a4.81 4.81 0 001.228.008l-.01-.029a.062.062 0 00-.004-.01s-.167-.133-.379-.377a3.842 3.842 0 01-.584-.897 3.382 3.382 0 01-.266-.862 3.176 3.176 0 01-.006-.972c.017-.12.04-.241.072-.366.093-.374.255-.772.507-1.192l.002-.003.241-.404c1.103-1.86 1.797-3.275 1.506-5.441a8.943 8.943 0 00-.078-.476zm4.668.576zm.013.203l.003.036v.01c0 .013-.003.025-.003.038 0-.014.003-.028.003-.043 0-.014-.002-.026-.003-.04zm-.012.275v.001l-.002.01-.002.014.004-.025zm1.353 5.928c-2.553 1.138-5.44 1.44-8.192 1.007-.14 1.108.384 2.218 1.214 2.93l.012.10c2.703-.433 4.975-2.168 6.966-3.946z',
  joomla: 'M16.719 14.759L14.22 17.26l-2.37 2.37-.462.466c-1.368 1.365-3.297 1.83-5.047 1.397-.327 1.424-1.604 2.49-3.13 2.49C1.438 23.983 0 22.547 0 20.772c0-1.518 1.055-2.789 2.469-3.123-.446-1.76.016-3.705 1.396-5.08l.179-.18 2.37 2.37-.184.181c-.769.779-.769 2.024 0 2.789.771.78 2.022.78 2.787 0l.465-.465 2.367-2.371 2.502-2.506 2.368 2.372zm.924 6.652c-1.822.563-3.885.12-5.328-1.318l-.18-.185 2.365-2.369.18.184c.771.768 2.018.768 2.787 0 .765-.765.769-2.01-.004-2.781l-.466-.465-2.365-2.37-2.502-2.503 2.37-2.369 2.499 2.505 2.367 2.37.464.464c1.365 1.36 1.846 3.278 1.411 5.021 1.56.224 2.759 1.56 2.759 3.18 0 1.784-1.439 3.21-3.209 3.21-1.545 0-2.851-1.096-3.135-2.565l-.013-.009zM6.975 9.461l2.508-2.505 2.37-2.369.462-.461C13.74 2.7 15.772 2.251 17.58 2.79c.212-1.561 1.555-2.775 3.179-2.775 1.772 0 3.211 1.437 3.211 3.209 0 1.631-1.216 2.978-2.79 3.186.519 1.799.068 3.816-1.35 5.234l-.182.184-2.369-2.369.184-.184c.769-.77.769-2.016 0-2.783-.766-.766-2.011-.768-2.781.003l-.462.461-2.37 2.369-2.505 2.502-2.37-2.366zm-2.653 2.647l-.461-.462C2.43 10.215 1.986 8.17 2.529 6.358 1.1 6.029.03 4.754.03 3.224.03 1.454 1.47.015 3.24.015c1.596 0 2.92 1.166 3.17 2.691 1.73-.405 3.626.065 4.979 1.415l.184.185-2.37 2.37-.183-.181c-.77-.765-2.016-.765-2.785 0-.771.781-.77 2.025-.005 2.79l.465.466 2.37 2.369 2.505 2.505-2.367 2.37-2.51-2.505-2.371-2.37v-.012z',
  drupal: 'M15.78 5.113C14.09 3.425 12.48 1.815 11.998 0c-.48 1.815-2.09 3.425-3.778 5.113-2.534 2.53-5.405 5.4-5.405 9.702a9.184 9.184 0 1018.368 0c0-4.303-2.871-7.171-5.405-9.702M6.72 16.954c-.563-.019-2.64-3.6 1.215-7.416l2.55 2.788a.218.218 0 01-.016.325c-.61.625-3.204 3.227-3.527 4.126-.066.186-.164.18-.222.177M12 21.677a3.158 3.158 0 01-3.158-3.159 3.291 3.291 0 01.787-2.087c.57-.696 2.37-2.655 2.37-2.655s1.774 1.988 2.367 2.649a3.09 3.09 0 01.792 2.093A3.158 3.158 0 0112 21.677m6.046-5.123c-.068.15-.223.398-.431.405-.371.014-.411-.177-.686-.583-.604-.892-5.864-6.39-6.848-7.455-.866-.935-.122-1.595.223-1.94C10.736 6.547 12 5.285 12 5.285s3.766 3.574 5.336 6.016c1.57 2.443 1.029 4.556.71 5.253',
  grav: 'M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm6.489 13.965c-1.251-.825-1.965-1.523-2.589-2.777-.427.859-1.421 2.135-3.098 3.139-.84 2.61-4.823 7.605-6.113 6.885-.381-.195-.452-.48-.367-.765.093-.704 1.566-2.34 1.566-2.34s.029.345.494 1.065c-.629-1.936 1.021-4.305 1.456-5.131.689-.209.734-1.095.734-1.095.046-1.364-.569-2.34-1.155-2.94.421.525.556 1.306.57 2.025v.255c-.029.601-.21 1.41-.585 1.41v.016c-.39-.016-.885.074-1.319.21l-.961.239s.51-.015.78.226c-.314.51-1.005 1.125-1.771 1.484-1.109.525-1.439-.51-.869-1.17.135-.165.285-.3.404-.404-.09-.09-.135-.21-.149-.36-.075-.345-.045-.78.45-1.485.09-.149.21-.3.345-.449l.015-.016.016-.015v-.015c.029-.046.074-.076.104-.12.57-.585 1.485-1.2 2.911-1.74 1.694-2.49 2.309-2.956 2.309-2.956.181-.179.511-.419.615-.479-.87-1.515-1.049-3.646-.824-4.215-.03.03-.046.06-.061.105.09-.195.135-.255.225-.36.24-.27 1.035-.42 1.336.18.15.315.18.735.18 1.035-.645-.029-1.215.69-1.215.69s.524-.24 1.186-.255c0 0 .179.164.389.449-.284.556-.779 1.725-.42 2.971.061.24.15.45.256.629.015.016.015.016.015.031l.03.029c.585.886 1.649.976 1.649.976-.495-.24-.915-.646-1.169-1.125-.136-.255-.227-.48-.271-.646-.285-1.08.135-1.725.375-2.145.54-.84 1.544-1.351 2.609-1.23 1.5.165 2.581 1.53 2.399 3.03-.104.915-.659 1.681-1.409 2.085.181.494-.015 1.08-.015 1.08.449.57.479.9.465 1.215-.585-.09-1.141.301-1.141.301s1.111-.256 1.756.314c.42.449.704.87.869 1.17.24.435 1.35.465 1.229 1.23-.135.779-.989.779-2.31-.09l.074-.151zm-4.824-4.61c-.22-.219-.574-.219-.795 0l-.465.468c-.222.21-.222.57 0 .796l.51.51c.222.225.577.21.795 0l.47-.466c.221-.225.221-.585 0-.794l-.515-.525v.011zm-2.205-.186c-.14.14-.14.368 0 .511.141.138.368.138.51 0 .14-.143.14-.371 0-.511-.142-.141-.369-.141-.51 0zm1.269-.252c.142-.139.142-.366 0-.51-.141-.138-.367-.138-.51 0-.139.144-.139.371 0 .51.142.142.369.142.51 0zm5.385-1.304c.591-1.131-.247-1.791-.825-2.332-.924-.87-1.846-1.245-2.9-.029-1.052 1.199-.383 2.609.58 3.284.96.69 2.535.226 3.135-.915l.01-.008zm-1.595-.463c-.372-.445.322-1.252.757-.77.8.89-.387 1.216-.757.77z',
  phpbb: 'M4.5826 11.9021q0 .5784-.1424 1.0634t-.4137.8409a1.85 1.85 0 0 1-.6763.5516q-.405.1959-.9299.1958-.4005 0-.7163-.2002t-.4938-.5117v2.1178H.8364a.87.87 0 0 1-.3114-.0578.82.82 0 0 1-.267-.1646.83.83 0 0 1-.1868-.258Q0 15.3278 0 15.1321v-2.9008q0-.5962.138-1.0945.1378-.4983.4315-.8543t.7474-.5517q.4539-.1957 1.0856-.1957.4805 0 .881.2002.4005.2003.6896.5294.2892.3292.4493.7564.1602.427.1602.881m-1.228.3115q0-.8542-.2802-1.2369-.2804-.3826-.8587-.3826-.4983 0-.7608.387t-.2625 1.01q0 .7208.3248 1.1123t.8586.3915q.436 0 .7075-.3515.2714-.3514.2714-.9298m5.4706 2.2334q-.3737 0-.5917-.2046t-.218-.5962v-1.9576q0-.2937-.0757-.5028-.0756-.2091-.2046-.3381a.81.81 0 0 0-.3026-.1913 1.09 1.09 0 0 0-.3693-.0623q-.1424 0-.2936.0578t-.2803.1913-.2091.347-.08.534v2.7228h-.3916q-.4272 0-.6229-.2135-.1958-.2136-.1958-.5873v-5.606h.3738q.427 0 .6362.2136t.2091.5517v1.3704q.0623-.1068.1735-.218a1.8 1.8 0 0 1 .2447-.2047 1.4 1.4 0 0 1 .2937-.1557.9.9 0 0 1 .3292-.0623q.9344 0 1.4504.5383.5161.5385.5161 1.5617v2.8118zm5.5595-2.5449q0 .5784-.1423 1.0634-.1425.485-.4138.8409a1.85 1.85 0 0 1-.6763.5516q-.4048.1959-.9298.1958-.4005 0-.7164-.2002t-.4938-.5117v2.1178h-.3737a.87.87 0 0 1-.3115-.0578.82.82 0 0 1-.267-.1646.83.83 0 0 1-.1868-.258q-.0711-.1514-.0712-.3471v-2.9008q0-.5962.138-1.0945.1379-.4983.4315-.8543.2937-.356.7475-.5517t1.0856-.1957q.4805 0 .8809.2002.4004.2003.6896.5294.2892.3292.4494.7564.1602.427.1601.881m-1.228.3115q0-.8542-.2802-1.2369-.2803-.3826-.8587-.3826-.4983 0-.7608.387t-.2625 1.01q0 .7208.3248 1.1123t.8587.3915q.4359 0 .7074-.3515t.2714-.9298m6.0312.347q0 .4627-.169.8186t-.4583.6007-.6674.3692-.7964.1246h-.6584q-1.6996 0-1.6996-1.4504V8.6009q.1068-.0357.3025-.0846a8.5 8.5 0 0 1 .4227-.0934 5.8 5.8 0 0 1 .4627-.0712q.2357-.0267.4227-.0267h.7385q.436 0 .8009.1112.3648.1113.6317.3204.267.209.4138.5072.1468.298.1468.6718 0 .5072-.258.832t-.7119.4582q.2314.0624.4271.2047.1958.1425.3426.3248.1469.1824.227.396.08.2135.08.4093m-1.3258-2.5182q0-.1958-.0846-.3337a.64.64 0 0 0-.218-.218q-.1335-.0801-.3025-.1157a1.64 1.64 0 0 0-.3381-.0356h-.4538a1.7 1.7 0 0 0-.2581.0223q-.1424.0222-.267.04v1.3792h.9344q.4093 0 .6985-.1735t.2892-.565m.0979 2.5538q0-.4983-.2536-.6852-.2536-.1868-.7875-.1868h-.97v1.2546q0 .1602.1513.2714.1513.1113.3826.1112h.4983q.4984 0 .7386-.209.2403-.2093.2403-.5562m6.04-.0356q0 .4627-.169.8186t-.4583.6007-.6674.3692-.7964.1246h-.6585q-1.6995 0-1.6995-1.4504V8.6009q.1068-.0357.3025-.0846a8.5 8.5 0 0 1 .4227-.0934 5.8 5.8 0 0 1 .4627-.0712q.2357-.0267.4226-.0267H21.9q.436 0 .8009.1112.3648.1113.6317.3204.267.209.4138.5072.1468.298.1468.6718 0 .5072-.258.832t-.7119.4582q.2313.0624.4271.2047a1.78 1.78 0 0 1 .3426.3248q.1468.1824.227.396.08.2135.08.4093m-1.3258-2.5182q0-.1958-.0846-.3337a.64.64 0 0 0-.218-.218q-.1335-.0801-.3025-.1157a1.64 1.64 0 0 0-.3382-.0356h-.4538a1.7 1.7 0 0 0-.258.0223q-.1425.0222-.267.04v1.3792h.9344q.4093 0 .6985-.1735t.2892-.565m.0978 2.5538q0-.4983-.2536-.6852-.2535-.1868-.7875-.1868h-.9699v1.2546q0 .1602.1513.2714.1512.1113.3826.1112h.4983q.4983 0 .7386-.209.2402-.2093.2402-.5562m.1952 3.0812h-.1223v-.7305h.2823q.135 0 .2032.0494.0684.0494.0684.1605 0 .099-.0558.1447-.0558.046-.1385.0547l.2086.3212h-.1384l-.1924-.3123h-.1151zm.1366-.4147a.8.8 0 0 0 .0657-.0026.14.14 0 0 0 .0548-.015.1.1 0 0 0 .0378-.0344q.0144-.022.0144-.0626 0-.0335-.0153-.053a.1.1 0 0 0-.0387-.03.16.16 0 0 0-.0521-.0132.6.6 0 0 0-.0558-.0026h-.1474v.2135zm.6546.037q0 .1484-.053.27a.63.63 0 0 1-.1439.2083.65.65 0 0 1-.2104.134.67.67 0 0 1-.2509.0477q-.1455 0-.267-.0503a.63.63 0 0 1-.2086-.1385.64.64 0 0 1-.1367-.209.69.69 0 0 1-.0494-.2621q0-.1482.053-.27a.63.63 0 0 1 .1439-.2082.65.65 0 0 1 .2113-.1341.68.68 0 0 1 .2535-.0477.67.67 0 0 1 .2509.0477.65.65 0 0 1 .2104.134.63.63 0 0 1 .1439.2083q.053.1218.053.27m-.1439 0a.6.6 0 0 0-.0395-.2205.52.52 0 0 0-.1097-.173.495.495 0 0 0-.1636-.112.51.51 0 0 0-.2015-.0397.52.52 0 0 0-.204.0397.49.49 0 0 0-.1646.112.52.52 0 0 0-.1097.173.6.6 0 0 0-.0395.2206q0 .113.036.2117a.52.52 0 0 0 .1033.173.49.49 0 0 0 .1628.1173q.0952.0432.2158.0432a.51.51 0 0 0 .2014-.0397.495.495 0 0 0 .1636-.112.52.52 0 0 0 .1097-.172q.0396-.0997.0396-.2215',
  nextcloud: 'M12.018 6.537c-2.5 0-4.6 1.712-5.241 4.015-.56-1.232-1.793-2.105-3.225-2.105A3.569 3.569 0 0 0 0 12a3.569 3.569 0 0 0 3.552 3.553c1.432 0 2.664-.874 3.224-2.106.641 2.304 2.742 4.016 5.242 4.016 2.487 0 4.576-1.693 5.231-3.977.569 1.21 1.783 2.067 3.198 2.067A3.568 3.568 0 0 0 24 12a3.569 3.569 0 0 0-3.553-3.553c-1.416 0-2.63.858-3.199 2.067-.654-2.284-2.743-3.978-5.23-3.977zm0 2.085c1.878 0 3.378 1.5 3.378 3.378 0 1.878-1.5 3.378-3.378 3.378A3.362 3.362 0 0 1 8.641 12c0-1.878 1.5-3.378 3.377-3.378zm-8.466 1.91c.822 0 1.467.645 1.467 1.468s-.644 1.467-1.467 1.468A1.452 1.452 0 0 1 2.085 12c0-.823.644-1.467 1.467-1.467zm16.895 0c.823 0 1.468.645 1.468 1.468s-.645 1.468-1.468 1.468A1.452 1.452 0 0 1 18.98 12c0-.823.644-1.467 1.467-1.467z',
  matomo: 'M6.664 15.37a3.336 3.336 0 0 1-3.332 3.332C1.495 18.702 0 17.208 0 15.37s1.495-3.333 3.332-3.333a3.338 3.338 0 0 1 3.332 3.333zm11.565-3.644a3.658 3.658 0 0 1-1.987.591 3.642 3.642 0 0 1-1.872-.529l.008.012a3.728 3.728 0 0 1-1.235-1.19l-2.612-3.693a.17.17 0 0 1-.027-.033A3.312 3.312 0 0 0 7.67 5.298a3.318 3.318 0 0 0-2.848 1.586.146.146 0 0 1-.021.028l-3.428 5.343a3.663 3.663 0 0 1 5.094 1.18.13.13 0 0 1 .015.018l2.756 3.869a3.305 3.305 0 0 0 2.699 1.38 3.31 3.31 0 0 0 2.711-1.379l.009-.013c.073-.103.137-.202.195-.305l1.442-2.255 1.935-3.024zm5.275 1.902l-.014-.028-.044-.066a1.109 1.109 0 0 0-.029-.044l-3.525-5.37c.024.168.052.335.052.51 0 .741-.219 1.457-.634 2.068l-2.803 4.38 1.416 2.179-.002.002a.131.131 0 0 1 .024.028 3.338 3.338 0 0 0 2.723 1.415A3.335 3.335 0 0 0 24 15.37c0-.613-.171-1.216-.496-1.742zm-7.262-1.666a3.336 3.336 0 0 0 3.332-3.333 3.336 3.336 0 0 0-3.332-3.332 3.336 3.336 0 0 0-3.332 3.332 3.338 3.338 0 0 0 3.332 3.333z',
  // Basit, özgün (CC0) geometrik ikonlar
  opencart: 'M2 3h3.5l1.8 11h11l2-7H6m2 14a1.6 1.6 0 100 3.2 1.6 1.6 0 000-3.2zm10 0a1.6 1.6 0 100 3.2 1.6 1.6 0 000-3.2z',
  mediawiki: 'M12 2a10 10 0 100 20 10 10 0 000-20zm0 2c2.5 0 4.7 1.3 6 3.3-1.7.7-3.7 1.1-6 1.1s-4.3-.4-6-1.1C7.3 5.3 9.5 4 12 4zM4 12c0-.4 0-.9.1-1.3 1.4 2.8 4.3 4.8 7.9 4.8s6.5-2 7.9-4.8c.1.4.1.9.1 1.3 0 4.4-3.6 8-8 8s-8-3.6-8-8zm.5-4.5C5.7 6.1 7.5 5.2 9.5 4.7c-.9 1-1.6 2.2-2.1 3.6-1.2-.2-2.2-.5-2.9-.8zM17.6 8.3c-.5-1.4-1.2-2.6-2.1-3.6 2 .5 3.8 1.4 5 2.8-.7.3-1.7.6-2.9.8z',
}

export default function DomainAppsPage() {
  const { t } = useTranslation(['DomainAppsPage', 'common'])
  const { id } = useParams()
  const navigate = useNavigate()
  const [alanAdi, setAlanAdi] = useState('')
  const [liste, setListe] = useState<Kurulu[]>([])
  const [yuk, setYuk] = useState(true)
  const [turler, setTurler] = useState<TurBilgi[]>([])
  const [hata, setHata] = useState<string | null>(null)
  const [mesgul, setMesgul] = useState<string | null>(null)

  const [sihirbazAcik, setSihirbazAcik] = useState(false)
  const [seciliTur, setSeciliTur] = useState<TurBilgi | null>(null)
  const [altDizin, setAltDizin] = useState('')
  const [alanlar, setAlanlar] = useState<Record<string, string>>({})
  const [kuruyor, setKuruyor] = useState(false)
  const [sonuc, setSonuc] = useState<Sonuc | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<{ alan_adi: string }>(`/domains/${id}`).then(r => setAlanAdi(r.data.alan_adi || '')).catch(() => {})
    api.get<TurBilgi[]>(`/domains/${id}/apps/turler`).then(r => setTurler(r.data || [])).catch(() => {})
  }, [id])

  const listele = useCallback(() => {
    if (!id) return
    setYuk(true)
    api.get<Kurulu[]>(`/domains/${id}/apps`).then(r => setListe(r.data || [])).catch(() => setListe([])).finally(() => setYuk(false))
  }, [id])
  useEffect(() => { listele() }, [listele])

  function turSec(tb: TurBilgi) {
    setSeciliTur(tb)
    const bos: Record<string, string> = {}
    tb.form_alanlari.forEach(fa => { bos[fa.anahtar] = '' })
    setAlanlar(bos)
    setAltDizin('')
    setHata(null)
  }

  async function kur(e: React.FormEvent) {
    e.preventDefault()
    if (!seciliTur) return
    setHata(null); setSonuc(null); setKuruyor(true)
    try {
      const { data } = await api.post<Sonuc>(`/domains/${id}/apps/${seciliTur.slug}/kur`, {
        alt_dizin: altDizin.trim(), alanlar,
      })
      setSonuc(data); setSihirbazAcik(false); setSeciliTur(null)
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:install_failed'))) }
    finally { setKuruyor(false) }
  }

  async function guncelle(k: Kurulu) {
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try { await api.post(`/domains/${id}/apps/${k.tur}/guncelle`, { dizin: k.dizin }); listele() }
    catch (err) { setHata(apiHata(err, t('DomainAppsPage:update_failed'))) }
    finally { setMesgul(null) }
  }

  async function sil(k: Kurulu) {
    if (!confirm(t('DomainAppsPage:confirm_delete', { ad: k.ad, yol: k.dizin }))) return
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try {
      await api.delete(`/domains/${id}/apps/${k.tur}`, { data: { dizin: k.dizin, db_sil: true } })
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:delete_failed'))) }
    finally { setMesgul(null) }
  }

  function yonetHedefi(k: Kurulu): string | null {
    if (k.tur === 'wordpress') return `/abonelikler/${id}/wordpress`
    if (k.tur === 'prestashop') return `/abonelikler/${id}/prestashop?dizin=${encodeURIComponent(k.dizin)}`
    return null
  }

  return (
    <div className="w-full px-6 py-6">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: alanAdi || t('DomainAppsPage:breadcrumb_subscription'), href: `/abonelikler/${id}` },
        { etiket: t('DomainAppsPage:breadcrumb_apps') },
      ]} />
      <div className="flex items-center justify-between gap-4 mb-6 flex-wrap">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainAppsPage:title')}</h1>
        <button onClick={() => { setSihirbazAcik(true); setSeciliTur(null) }} className="ta-primary-button">
          {t('DomainAppsPage:new_install_button')}
        </button>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {sonuc && (
        <div className="mb-4 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/15 p-4">
          <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300 mb-2">
            {t('DomainAppsPage:installed_ok', { version: sonuc.surum })}
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_site')}</span> <a href={sonuc.site_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.site_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_admin')}</span> <a href={sonuc.admin_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.admin_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_user')}</span> <span className="font-mono text-xs">{sonuc.admin_kullanici}</span></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_password')}</span> <span className="font-mono text-xs">{sonuc.admin_parola}</span></div>
          </div>
          <p className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">{t('DomainAppsPage:password_warning')}</p>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-6">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('DomainAppsPage:installed_title')}</h3>
        </div>
        {yuk ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:scanning')}</div>
        ) : liste.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:no_installations')}</div>
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700/60">
            {liste.map(k => {
              const key = k.tur + k.dizin
              const yonet = yonetHedefi(k)
              return (
                <div key={key} className="flex items-center justify-between gap-4 px-4 py-3 flex-wrap">
                  <div className="flex items-center gap-3 min-w-0">
                    <svg viewBox="0 0 24 24" className="w-6 h-6 text-slate-400 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[k.tur] || ICONS.wordpress} /></svg>
                    <div className="min-w-0">
                      <div className="font-medium text-slate-800 dark:text-slate-100">{k.ad} <span className="text-xs text-slate-400 font-mono">{k.dizin}</span></div>
                      <div className="text-xs text-slate-500 dark:text-slate-400">
                        {t('DomainAppsPage:version_label')} {k.surum || '—'}
                        {k.durum === 'eski' && <span className="text-amber-600 dark:text-amber-400 font-medium ml-1">{t('DomainAppsPage:status_update_to', { version: k.son_surum })}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <a href={k.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:admin_link')}</a>
                    {yonet && <button onClick={() => navigate(yonet)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:manage_link')}</button>}
                    <button disabled={!!mesgul} onClick={() => guncelle(k)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">
                      {mesgul === key ? '…' : t('DomainAppsPage:update_link')}
                    </button>
                    <button disabled={!!mesgul} onClick={() => sil(k)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{t('DomainAppsPage:delete')}</button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {sihirbazAcik && !seciliTur && (
        <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
          <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">{t('DomainAppsPage:pick_app_title')}</h3>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {turler.map(tb => (
              <button key={tb.slug} onClick={() => turSec(tb)} className="flex flex-col items-center gap-2 p-4 border border-slate-200 dark:border-slate-700 rounded-xl hover:border-brand-400 dark:hover:border-brand-500 hover:bg-slate-50 dark:hover:bg-slate-800">
                <svg viewBox="0 0 24 24" className="w-8 h-8 text-slate-500" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[tb.slug] || ICONS.wordpress} /></svg>
                <span className="text-sm font-medium text-slate-700 dark:text-slate-200">{tb.ad}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {sihirbazAcik && seciliTur && (
        <form onSubmit={kur} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 max-w-2xl">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{t('DomainAppsPage:install_title', { ad: seciliTur.ad })}</h3>
            <button type="button" onClick={() => setSeciliTur(null)} className="text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">{t('DomainAppsPage:pick_different')}</button>
          </div>
          <div className="mb-3">
            <label className="ta-label-sm">{t('DomainAppsPage:subdir_label')}</label>
            <input value={altDizin} onChange={e => setAltDizin(e.target.value)} placeholder={t('DomainAppsPage:subdir_placeholder')} className="ta-input ta-input-sm w-full font-mono" />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {seciliTur.form_alanlari.map(fa => (
              <label key={fa.anahtar} className="block">
                <span className="ta-label-sm">{fa.etiket}</span>
                <input
                  value={alanlar[fa.anahtar] || ''}
                  onChange={e => setAlanlar(a => ({ ...a, [fa.anahtar]: e.target.value }))}
                  required={fa.zorunlu}
                  placeholder={fa.yer_tutucu}
                  type={fa.tur === 'password' ? 'password' : fa.tur === 'email' ? 'email' : 'text'}
                  className="ta-input ta-input-sm w-full"
                />
              </label>
            ))}
          </div>
          <button disabled={kuruyor} className="ta-primary-button mt-3 w-full sm:w-auto">
            {kuruyor ? t('DomainAppsPage:installing_button') : t('DomainAppsPage:install_button')}
          </button>
        </form>
      )}
    </div>
  )
}
